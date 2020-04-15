package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
)

// Here are the patches for the game.
// Eventually, it will be imported as an emulator patch, so it is for work.
// Usage:
//   go run patch.go path/to/track03.bin.org track03.bin.patched

func must(err error) {
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
}

func main() {
	track03binSrc := os.Args[1]
	track03binDst := os.Args[2]

	fmt.Println("in:", track03binSrc, "out:", track03binDst)

	if track03binSrc == track03binDst {
		panic("same file")
	}

	r, err := os.Open(track03binSrc)
	must(err)
	defer r.Close()

	h := md5.New()
	_, err = io.Copy(h, r)
	must(err)
	hash := hex.EncodeToString(h.Sum(nil))

	fmt.Println(hash)
	if hash != "f3306ef9c874929685f0950a48cf5189" && // Disc2
		hash != "5b8092cd7faf8954f96b3e918c840ad3" { // Disc1
		panic("incorrect hash")
	}

	_, err = r.Seek(0, 0)
	must(err)

	bin, err := ioutil.ReadAll(r)
	must(err)
	r.Close()

	reps := [][]byte{
		[]byte("ATN3+MS=V34,1,14400,33600,14400,33600"),
		[]byte("ATM1\r                                "),
		[]byte("ca1203.mmcp6"),
		[]byte("192.168.0.10"),
		/*
			append([]byte("ca1203.mmcp6"), 0, 0),
			[]byte("153.121.44.150"),
		*/
	}

	for i := 0; i < len(reps); i += 2 {
		if len(reps[i]) != len(reps[i+1]) {
			panic(fmt.Sprint("mismach length", i, i+1))
		}
		bin = bytes.ReplaceAll(bin, reps[i], reps[i+1])
	}

	os.Remove(track03binDst)
	fmt.Println("Writing", track03binDst)
	err = ioutil.WriteFile(track03binDst, bin, 0644)
	must(err)
	fmt.Println("Done")
}
