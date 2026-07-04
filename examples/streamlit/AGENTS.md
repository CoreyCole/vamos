---
vamos_artifact: applet
applet:
  id: streamlit
  title: Streamlit Smoke Test
  kind: streamlit
  source_dir: .
  files_root: files
  route: /examples/streamlit
  app_route: /examples/streamlit/app/
  build_command: [uv, sync, --frozen]
  start_command: [./start.sh]
---

# Streamlit Smoke Test Applet

This directory is a minimal Vamos Streamlit applet for exercising process controls.

Rules for agents:

- Run `uv sync --frozen` after dependency changes.
- Run `PORT=8501 VAMOS_APP_FILES_ROOT="$PWD/files" ./start.sh` to test the app directly.
- Open `/examples/streamlit` in Vamos to verify the Workbench Start, Stop, and Restart controls.
- Restart should change the displayed process PID and process start time.
- Use `VAMOS_APP_FILES_ROOT` for durable applet files. Default: `./files`.
- Do not write durable app data outside `VAMOS_APP_FILES_ROOT`.
