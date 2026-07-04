from __future__ import annotations

import datetime as dt
import json
import os
import pathlib
import uuid

import streamlit as st

st.set_page_config(page_title="Vamos Streamlit Smoke Test", page_icon="🌰", layout="centered")


def app_files_root() -> pathlib.Path:
    root = pathlib.Path(os.environ.get("VAMOS_APP_FILES_ROOT", "files")).expanduser()
    root.mkdir(parents=True, exist_ok=True)
    return root


@st.cache_resource
def process_launch() -> dict[str, str | int]:
    root = app_files_root()
    started_at = dt.datetime.now(dt.timezone.utc).isoformat(timespec="seconds")
    launch = {
        "run_id": str(uuid.uuid4()),
        "pid": os.getpid(),
        "started_at": started_at,
        "port": os.environ.get("PORT", ""),
    }
    with (root / "launches.jsonl").open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(launch, sort_keys=True) + "\n")
    return launch


def launch_history() -> list[dict[str, object]]:
    path = app_files_root() / "launches.jsonl"
    if not path.exists():
        return []
    records: list[dict[str, object]] = []
    for line in path.read_text(encoding="utf-8").splitlines():
        try:
            records.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return records


launch = process_launch()
records = launch_history()

st.title("Streamlit applet smoke test")
st.caption("Use the Vamos Workbench controls to stop and restart this process.")

col1, col2, col3 = st.columns(3)
col1.metric("Process PID", str(launch["pid"]))
col2.metric("Launch records", str(len(records)))
col3.metric("Port", str(launch["port"] or "unknown"))

st.info(
    "Restart the applet from Vamos. When it comes back, the PID and process start time should change."
)

st.subheader("Process details")
st.json(launch)

if "clicks" not in st.session_state:
    st.session_state.clicks = 0
if st.button("Increment Streamlit session counter"):
    st.session_state.clicks += 1
st.write(f"Session counter: {st.session_state.clicks}")

st.subheader("Recent launch history")
if records:
    st.dataframe(records[-10:], use_container_width=True, hide_index=True)
else:
    st.write("No launches recorded yet.")

st.caption(f"Files root: `{app_files_root()}`")
