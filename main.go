package main

import (
	"crypto/md5"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

func existsFileDir(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func getMD5File(file string) (hash string, retErr error) {
	f, err := os.Open(file)
	if err != nil {
		retErr = err
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		retErr = err
		return
	}
	if st.IsDir() {
		retErr = errors.New("file is a directory")
		return
	}
	md5 := md5.New()
	io.Copy(md5, f)
	hash = fmt.Sprintf("%x", md5.Sum(nil))
	return
}

func getFilesFromDir(dir string) (retDirs []string, retErr error) {
	if !filepath.IsAbs(dir) {
		retErr = errors.New("filepath is not absolute")
		return
	}
	retDirs = make([]string, 0)
	retErr = filepath.Walk(dir, func(pth string, f os.FileInfo, err error) error {
		if rpth, rerr := filepath.Rel(dir, pth); !f.IsDir() && rerr == nil {
			retDirs = append(retDirs, rpth)
		}
		return nil
	})
	return
}

func filterFilesByRegExp(files []string, regex string) (retFiltered []string, retErr error) {
	crgx, err := regexp.Compile(regex)
	if err != nil {
		retErr = err
		return
	}
	retFiltered = make([]string, 0)
	for _, v := range files {
		if crgx.MatchString(v) {
			retFiltered = append(retFiltered, v)
		}
	}
	return
}

func getMD5Files(root string, files []string) map[string]string {
	retFiles := make(map[string]string)
	for _, v := range files {
		if hash, err := getMD5File(filepath.Join(root, v)); err == nil {
			retFiles[v] = hash
		}
	}
	return retFiles
}

func analyzeDirectory(rootDir, fileRegExp string) (files map[string]string, retErr error) {
	allFiles, err := getFilesFromDir(rootDir)
	if err != nil {
		retErr = err
		return
	}
	allFiles, err = filterFilesByRegExp(allFiles, fileRegExp)
	if err != nil {
		retErr = err
		return
	}
	files = getMD5Files(rootDir, allFiles)
	return
}

func main() {
	srcDir := flag.String("s", "", "source directory")
	destDir := flag.String("d", "", "destination directory")
	fileRegExp := flag.String("regexp", ".*", "file regexp")

	flag.Parse()

	rSrcDir, err := filepath.Abs(*srcDir)
	if err != nil {
		fmt.Println(err)
		return
	}
	rDestDir, err := filepath.Abs(*destDir)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(rDestDir)
	if exists, _ := existsFileDir(rSrcDir); !exists {
		fmt.Println("source directory does not exist")
		return
	}
	allSrcFiles, err := analyzeDirectory(rSrcDir, *fileRegExp)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(allSrcFiles)
	//fmt.Println(getMD5File(".git/COMMIT_EDITMSG"))
}
