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
	"strconv"
	"strings"
	"time"
)

const SCMD5FN = "smartcopy.md5"

var reSCMD5 = regexp.MustCompile("(.*)::([0-9a-fA-F]+)::(\\d*)::(.*)")

type myFile struct {
	Name    string
	Size    int64
	ModTime time.Time
	Hash    string
}

func (myf *myFile) getModTimeText() string {
	bts, err := myf.ModTime.MarshalText()
	if err != nil {
		return ""
	}
	return string(bts)
}

func (myf *myFile) setModTimeText(txt string) error {
	return myf.ModTime.UnmarshalText([]byte(txt))
}

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

func loadMD5File(filename string) (retMap map[string]*myFile, retErr error) {
	retMap = make(map[string]*myFile)
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
		if len(found) < 1 || len(found[0]) < 5 {
			continue
		}
		size, err := strconv.ParseInt(found[0][3], 10, 64)
		if err != nil {
			fmt.Printf("error: %s\n", err)
			continue
		}
		nmyf := &myFile{}
		nmyf.Name = found[0][1]
		nmyf.Size = size
		nmyf.Hash = found[0][2]
		err = nmyf.setModTimeText(found[0][4])
		if err != nil {
			fmt.Printf("error: %s\n", err)
			continue
		}
		retMap[nmyf.Name] = nmyf
	}
	return
}

func saveMD5File(filename string, hm map[string]*myFile) error {
	dasstr := ""
	for _, mf := range hm {
		dasstr += fmt.Sprintf("%s::%s::%d::%s\n", mf.Name, mf.Hash, mf.Size, mf.getModTimeText())
	}
	return ioutil.WriteFile(filename, []byte(dasstr), 0644)
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

func getFilesFromDir(dir string) (retDirs []*myFile, retErr error) {
	if !filepath.IsAbs(dir) {
		retErr = errors.New("filepath is not absolute")
		return
	}
	retDirs = make([]*myFile, 0)
	retErr = filepath.Walk(dir, func(pth string, f os.FileInfo, err error) error {
		if rpth, rerr := filepath.Rel(dir, pth); !f.IsDir() && rerr == nil {
			retDirs = append(retDirs, &myFile{rpth, f.Size(), f.ModTime(), ""})
		}
		return nil
	})
	return
}

func filterFilesByRegExp(files []*myFile, regex string) (retFiltered []*myFile, retErr error) {
	crgx, err := regexp.Compile(regex)
	if err != nil {
		retErr = err
		return
	}
	retFiltered = make([]*myFile, 0)
	for _, v := range files {
		if crgx.MatchString(v.Name) && strings.ToLower(v.Name) != SCMD5FN {
			retFiltered = append(retFiltered, v)
		}
	}
	return
}

func getMD5Files(root string, files []*myFile, fastmode, ignoremd5file bool) map[string]*myFile {
	retFiles := make(map[string]*myFile)
	scmd5fp := filepath.Join(root, SCMD5FN)
	if exists, _ := existsFileDir(scmd5fp); exists && !fastmode && !ignoremd5file {
		fmt.Println("reading smartcopy.md5")
		var err error
		retFiles, err = loadMD5File(scmd5fp)
		if err != nil {
			fmt.Printf("error: %s\n", err)
		}
		fmt.Printf("found %d hashes\n", len(retFiles))
		rrFls := make(map[string]*myFile)
		for _, rfl := range files {
			if rfval, rfok := retFiles[rfl.Name]; rfok {
				rrFls[rfl.Name] = rfval
				delete(retFiles, rfl.Name)
			}
		}
		for dfn, _ := range retFiles {
			fmt.Printf("file deleted: %s\n", dfn)
		}
		retFiles = rrFls
	}
	for _, v := range files {
		if !fastmode {
			fval, fok := retFiles[v.Name]
			if fok && v.Size == fval.Size && v.ModTime.Sub(fval.ModTime) == 0 {
				continue
			}
			fmt.Printf("hashing file: %s\n", v.Name)
			if hash, err := getMD5File(filepath.Join(root, v.Name)); err == nil {
				v.Hash = hash
			}
			if err := saveMD5File(scmd5fp, retFiles); err != nil {
				fmt.Printf("error: %s\n", err)
			}
			retFiles[v.Name] = v
		} else {
			fmt.Printf("setting file: %s\n", v.Name)
			v.Hash = "FM"
			retFiles[v.Name] = v
		}
	}
	if !fastmode {
		if err := saveMD5File(scmd5fp, retFiles); err != nil {
			fmt.Printf("error: %s\n", err)
		}
	}
	return retFiles
}

func analyzeDirectory(rootDir, fileRegExp string, fastmode, ignoremd5file bool) (files map[string]*myFile, retErr error) {
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

type myDiff struct {
	A, B    *myFile
	Reverse bool
}

func diffDirectory(dir1, dir2 map[string]*myFile, bidir bool) map[string]*myDiff {
	retDiff := make(map[string]*myDiff)
	for dn1, dv1 := range dir1 {
		if dv2, ok := dir2[dn1]; ok {
			if dv1.Hash != dv2.Hash {
				retDiff[dn1] = &myDiff{dv1, dv2, false}
				if dv1.ModTime.Sub(dv2.ModTime) < 0 {
					retDiff[dn1].Reverse = true
				}
			}
			continue
		}
		retDiff[dn1] = &myDiff{dv1, &myFile{}, false}
	}
	if bidir {
		for dn2, dv2 := range dir2 {
			if _, ok := dir1[dn2]; ok {
				continue
			}
			retDiff[dn2] = &myDiff{&myFile{}, dv2, true}
		}
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

func sumFloat64(f64 []float64) float64 {
	sum := float64(0)
	for _, v := range f64 {
		sum += v
	}
	return sum
}

func avgFloat64(f64 []float64) float64 {
	length := len(f64)
	if length < 1 {
		return float64(0)
	}
	sum := sumFloat64(f64)
	return sum / float64(length)
}

func copyFiles(src, dest string, files map[string]*myDiff) {
	filesCount := len(files)
	filesDone := 0
	allDurations := make([]float64, 0)
	if filesCount < 1 {
		return
	}
	fmt.Printf("copying %d files\n", filesCount)
	for fn, hsh := range files {
		progress := strPadding(fmt.Sprintf("%.1f%%", (float64(filesDone)/float64(filesCount))*100), 6)
		leftDurationStr := strPadding(formatF64Duration(float64(filesCount-filesDone)*avgFloat64(allDurations)), 8)
		filesDone++
		if len(hsh.B.Hash) > 1 {
			fmt.Printf("[%s] [%s] overwriting file: %s\n", progress, leftDurationStr, fn)
		} else {
			fmt.Printf("[%s] [%s] creating file: %s\n", progress, leftDurationStr, fn)
		}
		destFp := filepath.Join(dest, fn)
		destDir := filepath.Dir(destFp)
		if exists, _ := existsFileDir(destDir); !exists {
			if err := os.MkdirAll(destDir, 0755); err != nil {
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
	fmt.Printf("all files copied [%s]\n", strPadding(formatF64Duration(sumFloat64(allDurations)), 8))
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
		if err := os.MkdirAll(rDestDir, 0755); err != nil {
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
	diff := diffDirectory(allSrcFiles, allDestFiles, *diffOnly)
	if *diffOnly {
		fmt.Println(strings.Repeat("-", 40))
		fmt.Printf("found %d diff files\n", len(diff))
		for fn, hsh := range diff {
			toch := "new"
			if len(hsh.B.Hash) > 0 {
				toch = "change"
			}
			arrow := "->"
			if hsh.Reverse {
				arrow = "<-"
			}
			fmt.Printf("(%s): %s: %s %s %s\n", toch, fn, hsh.A.Hash, arrow, hsh.B.Hash)
		}
		return
	}

	fmt.Println(strings.Repeat("-", 40))

	if len(diff) < 1 {
		fmt.Println("no files to copy")
		return
	}
	copyFiles(rSrcDir, rDestDir, diff)
}
