package deployment

import (
	"strings"
)

// DetectFrameworks detects the primary framework for a list of directories based on the files present.
// Returns a map of directory path to framework name.
func DetectFrameworks(directories []string, files []string) map[string]string {
	frameworks := make(map[string]string)
	
	// Create a fast lookup map for files
	fileMap := make(map[string]bool)
	for _, f := range files {
		fileMap[f] = true
	}

	for _, dir := range directories {
		frameworks[dir] = "Other"
		
		// Determine prefix path for the directory
		basePath := ""
		if dir != "." {
			basePath = dir + "/"
		}

		// Node.js
		if fileMap[basePath+"package.json"] {
			frameworks[dir] = "Node.js"
			continue
		}
		
		// Go
		if fileMap[basePath+"go.mod"] {
			frameworks[dir] = "Go"
			continue
		}
		
		// Python
		if fileMap[basePath+"requirements.txt"] {
			frameworks[dir] = "Python"
			continue
		}
		// Also check for any .py file in this directory
		for f := range fileMap {
			if strings.HasPrefix(f, basePath) && strings.HasSuffix(f, ".py") {
				// Ensure it's directly in the directory, not in a nested sub-directory
				relPath := strings.TrimPrefix(f, basePath)
				if !strings.Contains(relPath, "/") {
					frameworks[dir] = "Python"
					break
				}
			}
		}
	}

	return frameworks
}
