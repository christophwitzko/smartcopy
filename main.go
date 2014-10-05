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
			} else if dv1.Hash == "FM" && dv2.Hash == "FM" && dv1.ModTime.Sub(dv2.ModTime) != 0 {
				fmt.Println(dv1.ModTime.Sub(dv2.ModTime))
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

func formatSize(pSize interface{}, mpl float64, suffix string) string {
	sizestr := []string{"", "K", "M", "G", "T"}
	var size float64
	switch cSize := pSize.(type) {
	case int64:
		size = float64(cSize)
	case int:
		size = float64(cSize)
	case float64:
		size = cSize
	default:
		return ""
	}
	size *= mpl
	isc := 0
	for {
		if isc > 3 || size < 1024 {
			break
		}
		size /= 1024.0
		isc++
	}
	if isc == 0 {
		return fmt.Sprintf("%d%s%s", int64(size), sizestr[isc], suffix)
	}
	return fmt.Sprintf("%.1f%s%s", size, sizestr[isc], suffix)
}

func formatSizeBits(size interface{}) string {
	return formatSize(size, 8, "Bits")
}

func formatSizeBytes(size interface{}) string {
	return formatSize(size, 1, "B")
}

type myFileStat struct {
	File     *myFile
	Duration float64
}

type myStat struct {
	FileRaw   map[string]*myDiff
	FileStats []*myFileStat
	Length    int
	Done      int
}

func (mst *myStat) Push(file *myFile, dur time.Duration) {
	mst.FileStats = append(mst.FileStats, &myFileStat{file, float64(dur)})
	mst.Done++
}

func (mst *myStat) DurationSum() float64 {
	sum := float64(0)
	for _, v := range mst.FileStats {
		sum += v.Duration
	}
	return sum
}

func (mst *myStat) DurationSumString() string {
	return formatF64Duration(mst.DurationSum())
}

func (mst *myStat) DurationAvg() float64 {
	length := len(mst.FileStats)
	if length < 1 {
		return float64(0)
	}
	sum := mst.DurationSum()
	return sum / float64(length)
}

func (mst *myStat) DurationAvgString() string {
	return formatF64Duration(mst.DurationAvg())
}

func (mst *myStat) DurationLeft() float64 {
	return float64(mst.Length-mst.Done) * mst.DurationAvg()
}

func (mst *myStat) DurationLeftString() string {
	return formatF64Duration(mst.DurationLeft())
}

func (mst *myStat) PercentLeft() float64 {
	return float64(mst.Done) / float64(mst.Length)
}

func (mst *myStat) PercentLeftString() string {
	return fmt.Sprintf("%.1f%%", mst.PercentLeft()*100)
}

func (mst *myStat) SizeSum() int64 {
	sum := int64(0)
	for _, v := range mst.FileStats {
		sum += v.File.Size
	}
	return sum
}

func (mst *myStat) SizeSumString() string {
	return formatSizeBytes(mst.SizeSum())
}

func (mst *myStat) SizeAvg() float64 {
	length := len(mst.FileStats)
	if length < 1 {
		return float64(0)
	}
	sum := mst.SizeSum()
	return float64(sum) / float64(length)
}

func (mst *myStat) SizeAvgString() string {
	return formatSizeBytes(mst.SizeAvg())
}

func (mst *myStat) RawSizeSum() int64 {
	sum := int64(0)
	for _, v := range mst.FileRaw {
		sum += v.A.Size
	}
	return sum
}

func (mst *myStat) RawSizeSumString() string {
	return formatSizeBytes(mst.RawSizeSum())
}

func (mst *myStat) RawSizeAvg() float64 {
	length := len(mst.FileRaw)
	if length < 1 {
		return float64(0)
	}
	sum := mst.RawSizeSum()
	return float64(sum) / float64(length)
}

func (mst *myStat) RawSizeAvgString() string {
	return formatSizeBytes(mst.RawSizeAvg())
}

func copyFiles(src, dest string, files map[string]*myDiff) {
	copyStat := &myStat{files, make([]*myFileStat, 0), len(files), 0}
	if copyStat.Length < 1 {
		return
	}
	fmt.Printf("copying %d files (%s/%s)\n", copyStat.Length, copyStat.RawSizeSumString(), copyStat.RawSizeAvgString())
	for fn, hsh := range files {
		cpType := "copying"
		if len(hsh.B.Hash) > 1 {
			cpType = "overwriting"
		}
		fmt.Printf("[%s][%s] %s file: %s\n", strPadding(copyStat.PercentLeftString(), 6), strPadding(copyStat.DurationLeftString(), 8), cpType, fn)
		destFp := filepath.Join(dest, fn)
		destDir := filepath.Dir(destFp)
		if exists, _ := existsFileDir(destDir); !exists {
			if err := os.MkdirAll(destDir, 0755); err != nil {
				fmt.Printf("could not create directory: %s\n", err)
				return
			}
			fmt.Printf("directory created: %s\n", filepath.Dir(fn))
		}
		cpCmd := exec.Command("cp", "-fp", filepath.Join(src, fn), destFp)
		tStart := time.Now()
		err := cpCmd.Run()
		copyStat.Push(hsh.A, time.Since(tStart))
		if err != nil {
			fmt.Printf("error: %s\n", err)
		}
	}
	fmt.Printf("[100.0%%][%s] all files copied (%s/%s)\n", strPadding(copyStat.DurationSumString(), 8), copyStat.SizeSumString(), copyStat.SizeAvgString())
}

func main() {
	srcDir := flag.String("s", "", "source directory")
	destDir := flag.String("d", "", "destination directory")
	fileRegExp := flag.String("regexp", ".*", "file regexp")
	diffOnly := flag.Bool("diff", false, "diff source vs. destination")
	fastMode := flag.Bool("fast", false, "enables fast mode (no hashing)")
	ignoreMD5File := flag.Bool("ignoremd5", false, "ignores the smartcopy.md5 file")
	analyzeOnly := flag.Bool("analyze", false, "analyzes only the source directory")
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
	if rSrcDir == rDestDir && !*analyzeOnly {
		fmt.Println("source directory and destination directory are the same")
		return
	}
	if exists, _ := existsFileDir(rSrcDir); !exists {
		fmt.Println("source directory does not exist")
		return
	}
	allSrcFiles, err := analyzeDirectory(rSrcDir, *fileRegExp, *fastMode, *ignoreMD5File)
	if err != nil {
		fmt.Println(err)
		return
	}
	if *analyzeOnly {
		return
	}
	fmt.Println(strings.Repeat("-", 40))
	if exists, _ := existsFileDir(rDestDir); !exists {
		if err := os.MkdirAll(rDestDir, 0755); err != nil {
			fmt.Println("could not create destination directory")
			return
		}
		fmt.Println("destination directory created")
		fmt.Println(strings.Repeat("-", 40))
	}
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
