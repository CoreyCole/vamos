package workflows

// Event persistence lives on Store so the Agent Chat adapter can be tested with
// in-memory fakes while DB-backed callers keep workspace timeline writes in one
// place. This file intentionally remains small until the UI projection slice
// adds richer workflow event payloads.
