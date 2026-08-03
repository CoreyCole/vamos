import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

import pytest


_H4_SHA = "5504217c3bb542794cfe71a4951279ce99b3dd92"
_checkout_text = os.environ.get("HERMES_EXACT_SESSION_CHECKOUT", "")
_expected_sha = os.environ.get("HERMES_EXACT_SESSION_H4_SHA", "")
_home = tempfile.mkdtemp(prefix="vamos-hermes-real-")
os.environ["HERMES_HOME"] = _home


def _git(checkout: Path, *args: str) -> str:
    return subprocess.check_output(
        ["git", "-C", str(checkout), *args], text=True, stderr=subprocess.STDOUT
    ).strip()


def pytest_configure(config):
    if not _checkout_text or not Path(_checkout_text).is_absolute():
        raise pytest.UsageError("HERMES_EXACT_SESSION_CHECKOUT must be an absolute path")
    checkout = Path(_checkout_text)
    if not checkout.is_dir() or not ((checkout / ".git").is_dir() or (checkout / ".git").is_file()):
        raise pytest.UsageError("HERMES_EXACT_SESSION_CHECKOUT is not a git checkout")
    if _expected_sha != _H4_SHA:
        raise pytest.UsageError("HERMES_EXACT_SESSION_H4_SHA does not equal the recorded H4 SHA")
    if _git(checkout, "rev-parse", "HEAD") != _H4_SHA:
        raise pytest.UsageError("Hermes checkout HEAD does not equal the recorded H4 SHA")
    dirty = _git(
        checkout,
        "status", "--porcelain", "--",
        "gateway", "hermes_cli", "agent", "tools",
        "hermes_state.py", "hermes_constants.py", "run_agent.py", "model_tools.py", "toolsets.py",
    )
    if dirty:
        raise pytest.UsageError("Hermes files imported by the real-core suite are dirty")
    site_packages = sorted((checkout / ".venv/lib").glob("python*/site-packages"))
    if len(site_packages) != 1:
        raise pytest.UsageError("Hermes exact-session checkout must have one prepared .venv")
    sys.path.append(str(site_packages[0]))


def pytest_sessionfinish(session, exitstatus):
    shutil.rmtree(_home, ignore_errors=True)
