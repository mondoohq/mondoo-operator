/*
Copyright 2016 The Rook Authors. All rights reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
	http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"fmt"
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

	return "", fmt.Errorf("mondoo operator root not found above directory %s", workingDirectory)
}

func ReadFile(filename string) string {
	rootDir, err := FindRootFolder()
	if err != nil {
		panic(err)
	}
	manifest := path.Join(rootDir, filename)
	zap.S().Infof("Reading file: %s", manifest)
	contents, err := os.ReadFile(manifest) //nolint:gosec
	if err != nil {
		panic(fmt.Errorf("failed to read file at %s. %v", manifest, err))
	}
	return string(contents)
}
