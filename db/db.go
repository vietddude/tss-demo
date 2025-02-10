package db

import (
	"fmt"
	"os"
)

func SaveToJSON(data []byte, path string) error {
	err := os.WriteFile(path, data, 0644)
	if err != nil {
		return err
	}

	fmt.Println("Data saved to", path)
	return nil
}

func LoadFromJSON(path string) ([]byte, error) {
	fileData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	fmt.Println("Data loaded from", path)
	return fileData, nil
}
