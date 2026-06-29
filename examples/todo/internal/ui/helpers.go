package ui

import "fmt"

func itemIDValue(id int64) string {
	return fmt.Sprintf("%d", id)
}

func itemTitleClass(completed bool) string {
	base := "flex-1 text-base"
	if completed {
		return base + " text-slate-500 line-through"
	}
	return base + " text-slate-100"
}
