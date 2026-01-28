# Simple process manager for linux platforms
### Usage
```bash
git clone https:github.com/DerrohXy/PMX
cd PMX
sudo bash install.sh

# After installation, to manage an array of processes
# Sample json file, config.json : [{"Name":"process1","Cmd":"venv/bin/python3","Args":["server.py","--port", "3001"],"Stdout":"server.log","AutoRestart":true}]

# Starting / restarting
pmx start config.json

# Stopping
pmx stop config.json

# Removing a process entry
pmx remove process1
```
