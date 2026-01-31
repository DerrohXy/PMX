package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
)

var (
	SERVICE_WORKING_DIR          = "/var/lib/pmx"
	PROCESS_LOG_FILEPATH         = SERVICE_WORKING_DIR + "/pmx-log.json"
	RUNNING_PROCESS_LOG_FILEPATH = SERVICE_WORKING_DIR + "/pmx-running-log.json"
	STOPPED_PROCESS_LOG_FILEPATH = SERVICE_WORKING_DIR + "/pmx-stopped-log.json"
)

type PMXProcess struct {
	Name        string   `json:"Name"`
	Cmd         string   `json:"Cmd"`
	Args        []string `json:"Args"`
	Stdout      *string  `json:"Stdout"`
	Stderr      *string  `json:"Stderr"`
	AutoRestart bool     `json:"AutoRestart"`
}

type ProcessStats struct {
	Pid  string `json:""`
	User string `json:""`
	RSS  string `json:""`
	CPU  string `json:""`
}

type PMXProcessLog map[string]PMXProcess

type PMXRunningProcessLog map[string]string

type PMXStoppedProcessLog map[string]bool

func init() {
	os.MkdirAll(SERVICE_WORKING_DIR, 0755)
}

func writeJson(filepath string, data interface{}) error {
	file, err := os.OpenFile(filepath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire exclusive lock: %w", err)
	}

	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	bytes, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return fmt.Errorf("json marshal error: %w", err)
	}

	if _, err := file.Write(bytes); err != nil {
		return fmt.Errorf("file write error: %w", err)
	}

	return nil
}

func readJson(filepath string, dest interface{}) error {
	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
		return fmt.Errorf("failed to acquire shared lock: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("json decode error: %w", err)
	}

	return nil
}

func loadProcessLog() (PMXProcessLog, error) {
	log := make(PMXProcessLog)

	err := readJson(PROCESS_LOG_FILEPATH, &log)

	if err != nil {
		return nil, err
	}

	err = writeJson(PROCESS_LOG_FILEPATH, log)
	if err != nil {
		return nil, err
	}

	return log, nil
}

func loadRunningProcessLog() (PMXRunningProcessLog, error) {
	log := make(PMXRunningProcessLog)

	err := readJson(RUNNING_PROCESS_LOG_FILEPATH, &log)

	if err != nil {
		return nil, err
	}

	err = writeJson(RUNNING_PROCESS_LOG_FILEPATH, log)
	if err != nil {
		return nil, err
	}

	return log, nil
}

func loadStoppedProcessLog() (PMXStoppedProcessLog, error) {
	log := make(PMXStoppedProcessLog)

	err := readJson(STOPPED_PROCESS_LOG_FILEPATH, &log)

	if err != nil {
		return nil, err
	}

	err = writeJson(STOPPED_PROCESS_LOG_FILEPATH, log)
	if err != nil {
		return nil, err
	}

	return log, nil
}

func getCmdArgs() []string {
	if len(os.Args) > 1 {
		return os.Args[1:]
	}

	return []string{}
}

func getProcessIdStats(pid string) (*ProcessStats, error) {
	pid_, err := strconv.ParseInt(pid, 10, 64)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid_), "-o", "user,rss,pcpu", "--no-headers")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("could not find process or ps command failed: %w", err)
	}

	fields := strings.Fields(string(output))
	if len(fields) < 3 {
		return nil, fmt.Errorf("unexpected output format from ps")
	}

	return &ProcessStats{
		Pid:  fmt.Sprintf("%d", pid_),
		User: fields[0],
		RSS:  fields[1],
		CPU:  fields[2],
	}, nil
}

func loadProcessLogFiles(process *PMXProcess) {
	if process.Stdout == nil || len(*process.Stdout) < 1 {
		stdout := fmt.Sprintf("%s/%s.out.log", SERVICE_WORKING_DIR, process.Name)
		process.Stdout = &stdout
	}

	if process.Stderr == nil || len(*process.Stderr) < 1 {
		stderr := fmt.Sprintf("%s/%s.err.log", SERVICE_WORKING_DIR, process.Name)
		process.Stderr = &stderr
	}
}

func startProcess(process PMXProcess) (int, error) {
	loadProcessLogFiles(&process)

	runningLog, err := loadRunningProcessLog()
	if err != nil {
		return 0, err
	}

	pid, isRunning := runningLog[process.Name]
	if isRunning {
		stopProcessId(pid)
	}

	processLog, err := loadProcessLog()
	if err != nil {
		return 0, err
	}

	cmd := exec.Command(process.Cmd, process.Args...)

	if process.Stdout != nil && len(*(process.Stdout)) > 0 {
		outfile, err := os.OpenFile(*(process.Stdout), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return 0, fmt.Errorf("failed to open stdout file: %w", err)
		}

		cmd.Stdout = outfile
	}

	if process.Stderr != nil && len(*(process.Stderr)) > 0 {
		errfile, err := os.OpenFile(*(process.Stderr), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return 0, fmt.Errorf("failed to open stderr file: %w", err)
		}

		cmd.Stderr = errfile
	}

	err = cmd.Start()
	if err != nil {
		return 0, fmt.Errorf("failed to start process: %w", err)
	}

	processLog[process.Name] = process

	pid_ := fmt.Sprintf("%d", cmd.Process.Pid)
	runningLog[process.Name] = pid_

	err = writeJson(PROCESS_LOG_FILEPATH, processLog)
	if err != nil {
		stopProcessId(pid_)

		return 0, fmt.Errorf("failed to update process log.")
	}

	err = writeJson(RUNNING_PROCESS_LOG_FILEPATH, runningLog)
	if err != nil {
		stopProcessId(pid_)

		return 0, fmt.Errorf("failed to update running log.")
	}

	return cmd.Process.Pid, nil
}

func stopProcessId(pid string) error {
	pid_, err := strconv.ParseInt(pid, 10, 64)
	if err != nil {
		return err
	}

	process_, err := os.FindProcess(int(pid_))
	if err != nil {

		return nil
	}

	err = process_.Signal(syscall.Signal(0))
	if err != nil {
		return nil
	}

	err = process_.Kill()
	if err != nil {
		return fmt.Errorf("failed to kill process %d: %w", pid_, err)
	}

	return nil
}

func stopProcess(process PMXProcess) error {
	runningLog, err := loadRunningProcessLog()
	if err != nil {
		return err
	}

	stoppedLog, err := loadStoppedProcessLog()
	if err != nil {
		return err
	}

	pid, isRunning := runningLog[process.Name]
	if !isRunning {
		return nil
	}

	err = stopProcessId(pid)
	if err != nil {

		return fmt.Errorf("failed to kill process %s: %w", pid, err)
	}

	stoppedLog[process.Name] = true
	err = writeJson(STOPPED_PROCESS_LOG_FILEPATH, stoppedLog)
	if err != nil {
		return fmt.Errorf("failed to update stopped log.")
	}

	delete(runningLog, process.Name)

	err = writeJson(RUNNING_PROCESS_LOG_FILEPATH, runningLog)
	if err != nil {
		return fmt.Errorf("failed to update running log.")
	}

	return nil
}

func runStart(args []string) error {
	processLog, err := loadProcessLog()
	if err != nil {
		return err
	}

	for _, configPath := range args {
		if strings.HasSuffix(configPath, ".json") {
			var processes []PMXProcess
			err = readJson(configPath, &processes)

			if err != nil {
				continue
			}

			for _, process := range processes {
				_, err := startProcess(process)

				if err != nil {
					log.Println(err.Error())
				}
			}
		} else {
			process, exists := processLog[configPath]
			if exists {
				_, err = startProcess(process)

				if err != nil {
					log.Println(err.Error())
				}
			}
		}
	}

	runLs()

	return nil
}

func runStop(args []string) error {
	runningLog, err := loadRunningProcessLog()
	if err != nil {
		return err
	}

	processLog, err := loadProcessLog()
	if err != nil {
		return err
	}

	for _, configPath := range args {
		if strings.HasSuffix(configPath, ".json") {
			var processes []PMXProcess
			err = readJson(configPath, &processes)

			if err != nil {
				continue
			}

			for _, process := range processes {
				err = stopProcess(process)

				if err != nil {
					log.Println(err.Error())
				}
			}
		} else {
			_, isRunning := runningLog[configPath]
			if isRunning {
				process, exists := processLog[configPath]
				if exists {
					err = stopProcess(process)

					if err != nil {
						log.Println(err.Error())
					}
				}
			}
		}
	}

	runLs()

	return nil
}

func runRemove(args []string) error {
	runningLog, err := loadRunningProcessLog()
	if err != nil {
		return err
	}

	processLog, err := loadProcessLog()
	if err != nil {
		return err
	}

	for _, processName := range args {
		process, exists := processLog[processName]
		if exists {
			_, isRunning := runningLog[processName]
			if isRunning {
				err = stopProcess(process)

				if err != nil {
					return err
				}
			}

			delete(processLog, process.Name)

			err = writeJson(PROCESS_LOG_FILEPATH, processLog)
			if err != nil {
				return fmt.Errorf("failed to update process log.")
			}
		}
	}

	runLs()

	return nil
}

func runLs() error {
	runningLog, err := loadRunningProcessLog()
	if err != nil {
		return err
	}

	processLog, err := loadProcessLog()
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Name\tPID\tStatus\tCPU\tMem\tUser\t")
	fmt.Fprintln(w, "----------\t----\t----\t----\t----\t----\t")

	for _, entry := range processLog {
		pid_ := "-"
		status_ := "offline"
		cpu_ := "-"
		mem_ := "-"
		user_ := "-"
		pid, isRunning := runningLog[entry.Name]
		if isRunning {
			pid_ = pid
			status_ = "online"

			stats, _ := getProcessIdStats(pid)
			if stats != nil {
				cpu_ = stats.CPU
				mem_ = stats.RSS
				user_ = stats.User
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t\n", entry.Name, pid_, status_, cpu_, mem_, user_)
	}

	w.Flush()

	return nil
}

func showLastLines(filepath string, lineCount int) {
	cmd := exec.Command("tail", "-n", strconv.Itoa(lineCount), filepath)

	output, err := cmd.Output()
	if err != nil {
		return
	}

	log.Println(string(output))
}

func tailFile(filepath string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("unable to open log file: %w", err)
	}

	defer file.Close()

	file.Seek(0, io.SeekEnd)

	buffer := make([]byte, 1024)
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			os.Stdout.Write(buffer[:n])
		}

		if err == io.EOF {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if err != nil {
			return fmt.Errorf("error reading logs: %w", err)
		}
	}
}

func runLogs(args []string) error {
	processLog, err := loadProcessLog()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("usage: pmx logs <process_name>")
	}

	processName := args[0]
	process, exists := processLog[processName]
	if !exists {
		return fmt.Errorf("process '%s' not found in registry", processName)
	}

	lineCount := 5
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "lines=") {
			val := strings.TrimPrefix(arg, "lines=")
			if i, err := strconv.Atoi(val); err == nil {
				lineCount = i
			}
		}
	}

	logType := "out"
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "type=") {
			val := strings.TrimPrefix(arg, "type=")
			val = strings.Trim(val, " ")
			if val == "err" {
				logType = "err"
			}
		}
	}

	logPath := ""
	switch logType {
	case "out":
		logPath = *process.Stdout
	case "err":
		logPath = *process.Stderr
	}

	if logPath == "" {
		return fmt.Errorf("no log files configured for process '%s'", processName)
	}

	log.Printf("--- Tailing logs for: %s (%s) ---\n", processName, logPath)

	showLastLines(logPath, lineCount)

	return tailFile(logPath)
}

func runMonitor() error {
	log.Println("PMX Monitor started. Press Ctrl+C to stop.")

	for {
		processLog, err := loadProcessLog()
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		runningLog, err := loadRunningProcessLog()
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		stoppedLog, err := loadStoppedProcessLog()
		if err != nil {
			return err
		}

		for name, config := range processLog {
			if !config.AutoRestart {
				continue
			}

			stopped, _ := stoppedLog[name]

			if stopped {
				continue
			}

			pidStr, isRunning := runningLog[name]

			shouldRestart := false

			if !isRunning {
				shouldRestart = true
			} else {
				pid, _ := strconv.Atoi(pidStr)
				process, _ := os.FindProcess(pid)

				err := process.Signal(syscall.Signal(0))
				if err != nil {
					shouldRestart = true
				}
			}

			if shouldRestart {
				log.Printf("[MONITOR] Process '%s' is down. Restarting...", name)
				_, err := startProcess(config)

				if err != nil {
					log.Printf("[MONITOR] Failed to restart '%s': %v", name, err)
				}
			}
		}

		time.Sleep(5 * time.Second)
	}
}

type ResponseItem struct {
	Name   string        `json:"Name"`
	Status string        `json:"Status"`
	Stats  *ProcessStats `json:"Stats,omitempty"`
}

func runServe(args []string) error {
	port := "8081"

	for _, arg := range args {
		if strings.HasPrefix(arg, "port=") {
			customPort := strings.TrimPrefix(arg, "port=")
			if _, err := strconv.Atoi(customPort); err == nil {
				port = customPort
			} else {
				log.Printf("[SERVE] Invalid port provided: %s. Using default: %s", customPort, port)
			}

			break
		}
	}

	http.HandleFunc("/status", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")

		runningLog, _ := loadRunningProcessLog()
		processLog, _ := loadProcessLog()

		processName := req.URL.Query().Get("name")
		if processName != "" {
			entry, exists := processLog[processName]
			if exists {
				status := "offline"
				var stats *ProcessStats
				if pid, isRunning := runningLog[processName]; isRunning {
					status = "online"
					stats, _ = getProcessIdStats(pid)
				}

				json.NewEncoder(res).Encode(ResponseItem{
					Name:   entry.Name,
					Status: status,
					Stats:  stats,
				})

			} else {
				json.NewEncoder(res).Encode(map[string]any{})
			}

		} else {
			var response []ResponseItem
			for name, entry := range processLog {
				status := "offline"
				var stats *ProcessStats
				if pid, isRunning := runningLog[name]; isRunning {
					status = "online"
					stats, _ = getProcessIdStats(pid)
				}
				response = append(response, ResponseItem{
					Name:   entry.Name,
					Status: status,
					Stats:  stats,
				})
			}

			json.NewEncoder(res).Encode(response)
		}
	})

	http.HandleFunc("/stop", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")

		processLog, _ := loadProcessLog()

		processName := req.URL.Query().Get("name")

		if processName != "" {
			entry, exists := processLog[processName]
			if exists {
				err := stopProcess(entry)
				if err != nil {
					json.NewEncoder(res).Encode(map[string]any{
						"Error": "Unable to stop process.",
					})
				}

				json.NewEncoder(res).Encode(map[string]any{
					"Message": "Process stopped.",
				})

			} else {
				json.NewEncoder(res).Encode(map[string]any{
					"Error": "Unknown process name.",
				})
			}

		} else {
			json.NewEncoder(res).Encode(map[string]any{
				"Error": "Process name is required.",
			})
		}
	})

	http.HandleFunc("/start", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")

		processLog, _ := loadProcessLog()

		processName := req.URL.Query().Get("name")

		if processName != "" {
			entry, exists := processLog[processName]
			if exists {
				_, err := startProcess(entry)
				if err != nil {
					json.NewEncoder(res).Encode(map[string]any{
						"Error": "Unable to start process.",
					})
				}

				json.NewEncoder(res).Encode(map[string]any{
					"Message": "Process started.",
				})

			} else {
				json.NewEncoder(res).Encode(map[string]any{
					"Error": "Unknown process name.",
				})
			}

		} else {
			json.NewEncoder(res).Encode(map[string]any{
				"Error": "Process name is required.",
			})
		}
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "PMX Monitoring Server is active. Visit /status for process details.")
	})

	log.Printf("--- PMX Web Server starting on http://0.0.0.0:%s ---\n", port)

	return http.ListenAndServe("0.0.0.0:"+port, nil)
}

func runHelp() {
	log.Println("PMX - Process Management Utility (Golang)")
	log.Println("\nUsage:")
	log.Println("  pmx <command> [arguments]")

	fmt.Println("\nCore Commands:")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(w, "  start <name|file.json>\tStarts a registered process or loads from a JSON config")
	fmt.Fprintln(w, "  stop <name|file.json>\tKills a running process by name or config")
	fmt.Fprintln(w, "  remove <name>\tRemoves a process from the registry. Stops it if running")
	fmt.Fprintln(w, "  ls\tLists all registered processes and their live stats")
	fmt.Fprintln(w, "  logs <name> [lines=N]\tTails logs for a process (default last 5 lines)")
	fmt.Fprintln(w, "  monitor\tStarts the auto-restart daemon (Runs in foreground)")
	w.Flush()

	log.Println("\nExamples:")
	log.Println("  pmx start my-api")
	log.Println("  pmx start ./config/services.json")
	log.Println("  pmx logs my-api lines=50")

	log.Println("\nSystem Integration:")
	log.Println("  To ensure auto-restart works, run 'pmx monitor' as a systemd service.")
}

func main() {
	args := getCmdArgs()
	var err error

	if len(args) < 1 {
		runHelp()

	} else {
		cmd := args[0]

		switch cmd {
		case "start":
			err = runStart(args[1:])

		case "stop":
			err = runStop(args[1:])

		case "remove":
			err = runRemove(args[1:])

		case "ls":
			err = runLs()

		case "logs":
			err = runLogs(args[1:])

		case "monitor":
			err = runMonitor()

		case "serve":
			err = runServe(args[1:])
			if err != nil {
				log.Fatalf("Server failed: %v", err)
			}

		default:
			runHelp()
		}
	}

	if err != nil {
		log.Println(err.Error())
	}
}
