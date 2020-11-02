package services

import (
	"bytes"
	"crawlab/constants"
	"crawlab/entity"
	"crawlab/model"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Unknwon/goconfig"
	"github.com/apex/log"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"runtime/debug"
	"strconv"
	"strings"
)

func GetScrapySpiderNames(s model.Spider) ([]string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command("scrapy", "list")
	cmd.Dir = s.Src
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Errorf(err.Error())
		debug.PrintStack()
		return []string{}, errors.New(stderr.String())
	}

	spiderNames := strings.Split(stdout.String(), "\n")

	var res []string
	for _, sn := range spiderNames {
		if sn != "" {
			res = append(res, sn)
		}
	}

	return res, nil
}

func GetScrapySettings(s model.Spider) (res []map[string]interface{}, err error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command("crawlab", "settings")
	cmd.Dir = s.Src
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Errorf(err.Error())
		log.Errorf(stderr.String())
		debug.PrintStack()
		return res, errors.New(stderr.String())
	}

	if err := json.Unmarshal([]byte(stdout.String()), &res); err != nil {
		log.Errorf(err.Error())
		debug.PrintStack()
		return res, err
	}

	return res, nil
}

func SaveScrapySettings(s model.Spider, settingsData []entity.ScrapySettingParam) (err error) {
	// 读取 scrapy.cfg
	cfg, err := goconfig.LoadConfigFile(path.Join(s.Src, "scrapy.cfg"))
	if err != nil {
		return
	}
	modName, err := cfg.GetValue("settings", "default")
	if err != nil {
		return
	}

	// 定位到 settings.py 文件
	arr := strings.Split(modName, ".")
	dirName := arr[0]
	fileName := arr[1]
	filePath := fmt.Sprintf("%s/%s/%s.py", s.Src, dirName, fileName)

	// 生成文件内容
	content := ""
	for _, param := range settingsData {
		var line string
		switch param.Type {
		case constants.String:
			line = fmt.Sprintf("%s = '%s'", param.Key, param.Value)
		case constants.Number:
			n := int64(param.Value.(float64))
			s := strconv.FormatInt(n, 10)
			line = fmt.Sprintf("%s = %s", param.Key, s)
		case constants.Boolean:
			if param.Value.(bool) {
				line = fmt.Sprintf("%s = %s", param.Key, "True")
			} else {
				line = fmt.Sprintf("%s = %s", param.Key, "False")
			}
		case constants.Array:
			arr := param.Value.([]interface{})
			var arrStr []string
			for _, s := range arr {
				arrStr = append(arrStr, s.(string))
			}
			line = fmt.Sprintf("%s = ['%s']", param.Key, strings.Join(arrStr, "','"))
		case constants.Object:
			value := param.Value.(map[string]interface{})
			var arr []string
			for k, v := range value {
				n := int64(v.(float64))
				s := strconv.FormatInt(n, 10)
				arr = append(arr, fmt.Sprintf("'%s': %s", k, s))
			}
			line = fmt.Sprintf("%s = {%s}", param.Key, strings.Join(arr, ","))
		}
		content += line + "\n"
	}

	// 写到 settings.py
	if err := ioutil.WriteFile(filePath, []byte(content), os.ModePerm); err != nil {
		return err
	}

	// 同步到GridFS
	if err := UploadSpiderToGridFsFromMaster(s); err != nil {
		return err
	}

	return
}

func GetScrapyItems(s model.Spider) (res []map[string]interface{}, err error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command("crawlab", "items")
	cmd.Dir = s.Src
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Errorf(err.Error())
		log.Errorf(stderr.String())
		debug.PrintStack()
		return res, errors.New(stderr.String())
	}

	if err := json.Unmarshal([]byte(stdout.String()), &res); err != nil {
		log.Errorf(err.Error())
		debug.PrintStack()
		return res, err
	}

	return res, nil
}

func SaveScrapyItems(s model.Spider, itemsData []entity.ScrapyItem) (err error) {
	// 读取 scrapy.cfg
	cfg, err := goconfig.LoadConfigFile(path.Join(s.Src, "scrapy.cfg"))
	if err != nil {
		return
	}
	modName, err := cfg.GetValue("settings", "default")
	if err != nil {
		return
	}

	// 定位到 settings.py 文件
	arr := strings.Split(modName, ".")
	dirName := arr[0]
	fileName := "items"
	filePath := fmt.Sprintf("%s/%s/%s.py", s.Src, dirName, fileName)

	// 生成文件内容
	content := ""
	content += "import scrapy\n"
	content += "\n\n"
	for _, item := range itemsData {
		content += fmt.Sprintf("class %s(scrapy.Item):\n", item.Name)
		for _, field := range item.Fields {
			content += fmt.Sprintf("    %s = scrapy.Field()\n", field)
		}
		content += "\n\n"
	}

	// 写到 settings.py
	if err := ioutil.WriteFile(filePath, []byte(content), os.ModePerm); err != nil {
		return err
	}

	// 同步到GridFS
	if err := UploadSpiderToGridFsFromMaster(s); err != nil {
		return err
	}

	return
}

func GetScrapyPipelines(s model.Spider) (res []string, err error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command("crawlab", "pipelines")
	cmd.Dir = s.Src
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Errorf(err.Error())
		log.Errorf(stderr.String())
		debug.PrintStack()
		return res, errors.New(stderr.String())
	}

	if err := json.Unmarshal([]byte(stdout.String()), &res); err != nil {
		log.Errorf(err.Error())
		debug.PrintStack()
		return res, err
	}

	return res, nil
}

func GetScrapySpiderFilepath(s model.Spider, spiderName string) (res string, err error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command("crawlab", "find_spider_filepath", "-n", spiderName)
	cmd.Dir = s.Src
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Errorf(err.Error())
		log.Errorf(stderr.String())
		debug.PrintStack()
		return res, err
	}

	res = strings.Replace(stdout.String(), "\n", "", 1)

	return res, nil
}

func CreateScrapySpider(s model.Spider, name string, domain string, template string) (err error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command("scrapy", "genspider", name, domain, "-t", template)
	cmd.Dir = s.Src
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Errorf(err.Error())
		log.Errorf("stdout: " + stdout.String())
		log.Errorf("stderr: " + stderr.String())
		debug.PrintStack()
		return err
	}

	return
}

func CreateScrapyProject(s model.Spider) (err error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command("scrapy", "startproject", s.Name, s.Src)
	cmd.Dir = s.Src
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Errorf(err.Error())
		log.Errorf("stdout: " + stdout.String())
		log.Errorf("stderr: " + stderr.String())
		debug.PrintStack()
		return err
	}

	return
}
