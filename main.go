package main

import (
	"crypto/md5"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const SCMD5FN = "smartcopy.md5"

var reSCMD5 = regexp.MustCompile("(.*)(::)([0-9a-fA-F]+)")

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

func loadMD5File(filename string) (retMap map[string]string, retErr error) {
	retMap = make(map[string]string)
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		retErr = err
		return
	}
	sfastr := strings.Split(string(data), "\n")
	if len(sfastr) < 1 {
		retErr = errors.New("empty MD5 file")
		return
	}
	for _, ln := range sfastr {
		found := reSCMD5.FindAllStringSubmatch(ln, -1)
		if len(found) < 1 || len(found[0]) < 4 {
			continue
		}
		retMap[found[0][1]] = found[0][3]
	}
	return
}

func saveMD5File(filename string, hm map[string]string) error {
	dasstr := ""
	for fn, hs := range hm {
		dasstr += fmt.Sprintf("%s::%s\n", fn, hs)
	}

	return ioutil.WriteFile(filename, []byte(dasstr), 0700)
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
		if crgx.MatchString(v) && strings.ToLower(v) != SCMD5FN {
			retFiltered = append(retFiltered, v)
		}
	}
	return
}

func getMD5Files(root string, files []string, fastmode, ignoremd5file bool) map[string]string {
	retFiles := make(map[string]string)
	scmd5fp := filepath.Join(root, SCMD5FN)
	if exists, _ := existsFileDir(scmd5fp); exists && !fastmode && !ignoremd5file {
		fmt.Println("reading smartcopy.md5")
		var err error
		retFiles, err = loadMD5File(scmd5fp)
		if err != nil {
			fmt.Printf("error: %s\n", err)
		}
		fmt.Printf("found %d hashes\n", len(retFiles))
	}
	for _, v := range files {
		if !fastmode {
			if _, ok := retFiles[v]; !ok {
				fmt.Printf("hashing file: %s\n", v)
				if hash, err := getMD5File(filepath.Join(root, v)); err == nil {
					retFiles[v] = hash
				}
			}
			if err := saveMD5File(scmd5fp, retFiles); err != nil {
				fmt.Printf("error: %s\n", err)
			}
			continue
		}
		fmt.Printf("setting file: %s\n", v)
		retFiles[v] = "FM"
	}
	return retFiles
}

func analyzeDirectory(rootDir, fileRegExp string, fastmode, ignoremd5file bool) (files map[string]string, retErr error) {
	fmt.Printf("analyzing directory: %s\n", rootDir)
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
	fmt.Printf("found %d files\n", len(allFiles))
	files = getMD5Files(rootDir, allFiles, fastmode, ignoremd5file)
	return
}

func diffDirectory(dir1, dir2 map[string]string) map[string][]string {
	retDiff := make(map[string][]string)
	for dn1, dv1 := range dir1 {
		if dv2, ok := dir2[dn1]; ok {
			if dv1 != dv2 {
				retDiff[dn1] = []string{dv1, dv2}
			}
			continue
		}
		retDiff[dn1] = []string{dv1, ""}
	}
	return retDiff
}

func strPadding(instr string, padlen int) string {
	pdiff := padlen - len(instr)
	if pdiff <= 0 {
		return instr
	}
	return strings.Repeat(" ", pdiff) + instr
}

func formatF64Duration(dur float64) string {
	d := dur / float64(time.Second)
	if d < 3600 {
		return fmt.Sprintf("%d:%.2d", int(d/60), int(d)%60)
	}
	return fmt.Sprintf("%d:%.2d:%.2d", int(d/3600), int(d/60)%60, int(d)%60)
}

func sumF64Duration(durs []float64) float64 {
	sum := float64(0)
	for _, v := range durs {
		sum += v
	}
	return sum
}

func avgF64Duration(durs []float64) float64 {
	length := len(durs)
	if length < 1 {
		return float64(0)
	}
	sum := sumF64Duration(durs)
	return sum / float64(length)
}

func copyFiles(src, dest string, files map[string][]string) {
	filesCount := len(files)
	filesDone := 0
	allDurations := make([]float64, 0)
	if filesCount < 1 {
		return
	}
	for fn, hsh := range files {
		progress := strPadding(fmt.Sprintf("%.1f%%", (float64(filesDone)/float64(filesCount))*100), 6)
		leftDurationStr := strPadding(formatF64Duration(float64(filesCount-filesDone)*avgF64Duration(allDurations)), 8)
		filesDone++
		if len(hsh[1]) > 1 {
			fmt.Printf("[%s] [%s] overwriting file: %s\n", progress, leftDurationStr, fn)
		} else {
			fmt.Printf("[%s] [%s] creating file: %s\n", progress, leftDurationStr, fn)
		}
		destFp := filepath.Join(dest, fn)
		destDir := filepath.Dir(destFp)
		if exists, _ := existsFileDir(destDir); !exists {
			if err := os.MkdirAll(destDir, 0700); err != nil {
				fmt.Printf("could not create directory: %s\n", err)
				return
			}
			fmt.Printf("directory created: %s\n", filepath.Dir(fn))
		}
		cpCmd := exec.Command("cp", "-f", filepath.Join(src, fn), destFp)
		tStart := time.Now()
		err := cpCmd.Run()
		allDurations = append(allDurations, float64(time.Since(tStart)))
		if err != nil {
			fmt.Printf("error: %s\n", err)
		}
	}
	fmt.Printf("all files copied [%s]\n", strPadding(formatF64Duration(sumF64Duration(allDurations)), 8))
}

func main() {
	srcDir := flag.String("s", "", "source directory")
	destDir := flag.String("d", "", "destination directory")
	fileRegExp := flag.String("regexp", ".*", "file regexp")
	diffOnly := flag.Bool("diff", false, "diff source vs. destination")
	fastMode := flag.Bool("fast", false, "enables fast mode (no hashing)")
	ignoreMD5File := flag.Bool("ignoremd5", false, "ignores the smartcopy.md5 file and updates it")

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
	if rSrcDir == rDestDir {
		fmt.Println("source directory and destination directory are the same")
		return
	}
	if exists, _ := existsFileDir(rSrcDir); !exists {
		fmt.Println("source directory does not exist")
		return
	}

	if exists, _ := existsFileDir(rDestDir); !exists {
		if err := os.MkdirAll(rDestDir, 0700); err != nil {
			fmt.Println("could not create destination directory")
			return
		}
		fmt.Println("directory destination created")
	}

	fmt.Println(strings.Repeat("-", 40))
	allSrcFiles, err := analyzeDirectory(rSrcDir, *fileRegExp, *fastMode, *ignoreMD5File)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(strings.Repeat("-", 40))
	allDestFiles, err := analyzeDirectory(rDestDir, *fileRegExp, *fastMode, *ignoreMD5File)
	if err != nil {
		fmt.Println(err)
		return
	}
	diff := diffDirectory(allSrcFiles, allDestFiles)
	if *diffOnly {
		fmt.Println(strings.Repeat("-", 40))
		fmt.Printf("found %d diff files\n", len(diff))
		for fn, hsh := range diff {
			if len(hsh[1]) > 0 {
				fmt.Printf("%s\nchange: %s -> %s\n\n", fn, hsh[0], hsh[1])
				continue
			}
			fmt.Printf("%s\nnew file: %s\n\n", fn, hsh[0])
		}
		return
	}

	fmt.Println(strings.Repeat("-", 40))

	if len(diff) < 1 {
		fmt.Println("no files to copy")
		return
	}

	fmt.Printf("copying %d files\n", len(diff))
	copyFiles(rSrcDir, rDestDir, diff)
}
