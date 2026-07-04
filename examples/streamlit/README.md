# Streamlit Smoke Test

A tiny Streamlit applet used to verify Vamos HTTP applet process controls and Streamlit proxy support.

## Run in Vamos

1. Start Vamos.
2. Open `/examples/streamlit`.
3. Use the applet controls in the Workbench header:
   - **Start** should launch the Streamlit process.
   - **Stop** should stop it and show the stopped state.
   - **Restart** should relaunch it with a new PID and process start time.

The app writes launch records to `files/launches.jsonl` through `VAMOS_APP_FILES_ROOT`.

## Run directly

```bash
uv sync --frozen
PORT=8501 VAMOS_APP_FILES_ROOT="$PWD/files" ./start.sh
```

Then open <http://127.0.0.1:8501/>.
