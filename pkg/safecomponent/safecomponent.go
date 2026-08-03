package safecomponent

import "fmt"

const MaxLength = 128

func ValidateBounded(value string) error {
	if len(value) == 0 || len(value) > MaxLength || value == "." || value == ".." {
		return fmt.Errorf("unsafe bounded component %q", value)
	}
	for i := 0; i < len(value); i++ {
		character := value[i]
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' {
			continue
		}
		return fmt.Errorf("unsafe bounded component %q", value)
	}
	return nil
}
