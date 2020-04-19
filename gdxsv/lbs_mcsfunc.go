package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	mcsFuncClientCreated time.Time
	mcsFuncClientCache   *http.Client
	mcsFuncRequestTime   = map[string]time.Time{}
)

func getMcsFuncClient() (*http.Client, error) {
	if mcsFuncClientCache != nil && time.Since(mcsFuncClientCreated).Minutes() <= 30.0 {
		return mcsFuncClientCache, nil
	}

	jsonKey, err := ioutil.ReadFile(conf.McsFuncKey)
	if err != nil {
		return nil, err
	}

	jwtConf, err := google.JWTConfigFromJSON(jsonKey)
	if err != nil {
		return nil, err
	}
	jwtConf.PrivateClaims = map[string]interface{}{
		"target_audience": conf.McsFuncURL,
	}
	jwtConf.UseIDToken = true
	mcsFuncClientCache = jwtConf.Client(oauth2.NoContext)
	mcsFuncClientCreated = time.Now()
	return mcsFuncClientCache, nil
}

func McsFuncEnabled() bool {
	return conf.McsFuncKey != "" && conf.McsFuncURL != ""
}

func McsFuncAlloc(region string) error {
	if time.Since(mcsFuncRequestTime["alloc/"+region]).Seconds() <= 10 {
		return nil
	}
	mcsFuncRequestTime["alloc/"+region] = time.Now()

	client, err := getMcsFuncClient()
	if err != nil {
		return err
	}

	resp, err := client.Get(conf.McsFuncURL + fmt.Sprintf("/alloc?region=%s", region))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	glog.Info(string(body))
	return nil
}

func GoMcsFuncAlloc(region string) {

	go func() {
		err := McsFuncAlloc(region)
		if err != nil {
			glog.Error(err)
		}
	}()
}

func McsFuncList() error {
	client, err := getMcsFuncClient()
	if err != nil {
		return err
	}

	resp, err := client.Get(conf.McsFuncURL + "/alloc?region=asia-northeast1")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	glog.Info(string(body))
	return nil
}

func GoMcsFuncList() {
	go func() {
		err := McsFuncList()
		if err != nil {
			glog.Error(err)
		}
	}()
}
