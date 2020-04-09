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

	if hash != "f3306ef9c874929685f0950a48cf5189" {
		panic("incorrect hash")
	}

	_, err = r.Seek(0, 0)
	must(err)

	bin, err := ioutil.ReadAll(r)
	must(err)
	r.Close()

	atms := [][]byte{
		[]byte("ATN3+MS=V34,1,14400,33600,14400,33600"),
		[]byte("ATM1\r                                "),
		[]byte("AT+MS=V34,1,33600,33600,33600,33600"),
		[]byte("ATM1\r                              "),
		[]byte("AT+MS=V34,1,28800,33600,28800,33600"),
		[]byte("ATM1\r                              "),
		[]byte("AT+MS=V34,1,14400,33600,14400,33600"),
		[]byte("ATM1\r                              "),
		[]byte("ca1203.mmcp6"),
		[]byte("192.168.0.10"),
	}

	for i := 0; i < len(atms); i += 2 {
		if len(atms[i]) != len(atms[i+1]) {
			panic(fmt.Sprint("mismach length", i, i+1))
		}
		bin = bytes.ReplaceAll(bin, atms[i], atms[i+1])
	}

	os.Remove(track03binDst)
	fmt.Println("Writing", track03binDst)
	err = ioutil.WriteFile(track03binDst, bin, 0x644)
	must(err)
	fmt.Println("Done")
}
