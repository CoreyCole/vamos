package ui

type Item struct {
	ID        int64
	Title     string
	Completed bool
}

type PageData struct {
	Items   []Item
	Message string
}
