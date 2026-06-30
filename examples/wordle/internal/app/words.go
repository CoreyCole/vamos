package app

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
)

type WordList struct {
	Ordered []string
	Words   map[string]struct{}
	Version string
}

func LoadWordList(path string) (WordList, error) {
	file, err := os.Open(path)
	if err != nil {
		return WordList{}, fmt.Errorf("open word list: %w", err)
	}
	defer file.Close()

	var ordered []string
	words := map[string]struct{}{}
	hash := sha256.New()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		word, err := NormalizeGuess(scanner.Text())
		if err != nil {
			return WordList{}, fmt.Errorf("invalid word %q: %w", scanner.Text(), err)
		}
		if _, exists := words[word]; exists {
			continue
		}
		ordered = append(ordered, word)
		words[word] = struct{}{}
		_, _ = hash.Write([]byte(word + "\n"))
	}
	if err := scanner.Err(); err != nil {
		return WordList{}, fmt.Errorf("read word list: %w", err)
	}
	if len(ordered) == 0 {
		return WordList{}, errors.New("word list is empty")
	}
	return WordList{
		Ordered: ordered,
		Words:   words,
		Version: fmt.Sprintf("sha256:%x", hash.Sum(nil)),
	}, nil
}

func (w WordList) Contains(word string) bool {
	_, ok := w.Words[strings.ToLower(word)]
	return ok
}
