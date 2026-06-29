---
vamos_artifact: applet
applet:
  id: pickleball
  title: Pickleball Tournament
  files_root: files
  current_app_dir: files/apps/current
  iterations_dir: files/apps/iterations
  route: /examples/pickleball
  app_route: /examples/pickleball/app/
  start_command: [go, run, .]
  health_path: /healthz
---

# Pickleball Applet Directory

This directory is a long-running Vamos applet, not only a static documentation directory.

The committed applet source lives in `files/apps/current/`. Generated applet iterations live in `files/apps/iterations/`. Vamos runs the current app as a local Go HTTP process and proxies it at `/examples/pickleball/app/`.

Future generic Thoughts rendering should use this frontmatter to detect applet directories and choose an applet Workbench view instead of a normal markdown-directory view.
