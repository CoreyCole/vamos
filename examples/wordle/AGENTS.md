---
vamos_artifact: applet
applet:
  id: wordle
  title: Server Wordle
  files_root: files
  current_app_dir: files/apps/current
  iterations_dir: files/apps/iterations
  route: /examples/wordle
  app_route: /examples/wordle/app/
  start_command: [go, run, .]
  health_path: /healthz
---

# Wordle Applet Directory

This directory is a long-running Vamos applet, not only a static documentation directory.

The committed applet source lives in `files/apps/current/`. The applet runs as a local Go HTTP process and is proxied through Vamos at `/examples/wordle/app/`.

Future generic Thoughts rendering should use this frontmatter to detect applet directories and choose an applet Workbench view instead of a normal markdown-directory view.
