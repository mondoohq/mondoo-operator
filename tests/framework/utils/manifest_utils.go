package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"go.uber.org/zap"
)

func FindRootFolder() (string, error) {
	const folderToFind = "tests"
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to find current working directory. %v", err)
	}
	parentPath := workingDirectory
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to find user home directory. %v", err)
	}
	for parentPath != userHome {
		_, err := os.Stat(path.Join(parentPath, folderToFind))
		if os.IsNotExist(err) {
			parentPath = filepath.Dir(parentPath)
			continue
		}
		return parentPath, nil
	}

	return "", fmt.Errorf("Mondoo operator root not found above directory %s", workingDirectory)
}

func ReadFile(filename string) string {
	rootDir, err := FindRootFolder()
	if err != nil {
		panic(err)
	}
	manifest := path.Join(rootDir, filename)
	zap.S().Infof("Reading file: %s", manifest)
	contents, err := ioutil.ReadFile(manifest)
	if err != nil {
		panic(fmt.Errorf("failed to read file at %s. %v", manifest, err))
	}
	return string(contents)
}
