package workspaces

import (
	"bufio"
	"os"
	"strings"
)

type FileLogTailer struct{}

func NewFileLogTailer() FileLogTailer { return FileLogTailer{} }

func (FileLogTailer) Tail(path string, lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	if lines > 2000 {
		lines = 2000
	}

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	ring := make([]string, lines)
	count := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		ring[count%lines] = scanner.Text()
		count++
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	start := 0
	if count > lines {
		start = count % lines
		count = lines
	}

	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, ring[(start+i)%lines])
	}
	return strings.Join(out, "\n"), nil
}
