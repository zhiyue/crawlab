package spider_handler

import (
	"crawlab/constants"
	"crawlab/database"
	"crawlab/model"
	"crawlab/services/local_node"
	"crawlab/utils"
	"fmt"
	"github.com/apex/log"
	"github.com/globalsign/mgo/bson"
	"github.com/satori/go.uuid"
	"github.com/spf13/viper"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
)

const (
	Md5File = "md5.txt"
)

type SpiderSync struct {
	Spider model.Spider
}

func (s *SpiderSync) CreateMd5File(md5 string) {
	path := filepath.Join(viper.GetString("spider.path"), s.Spider.Name)
	utils.CreateDirPath(path)

	fileName := filepath.Join(path, Md5File)
	file := utils.OpenFile(fileName)
	defer utils.Close(file)
	if file != nil {
		if _, err := file.WriteString(md5 + "\n"); err != nil {
			log.Errorf("file write string error: %s", err.Error())
			debug.PrintStack()
		}
	}
}

func (s *SpiderSync) CheckIsScrapy() {
	if s.Spider.Type == constants.Configurable {
		return
	}
	if viper.GetString("setting.checkScrapy") != "Y" {
		return
	}
	s.Spider.IsScrapy = utils.Exists(path.Join(s.Spider.Src, "scrapy.cfg"))
	if s.Spider.IsScrapy {
		s.Spider.Cmd = "scrapy crawl"
	}
	if err := s.Spider.Save(); err != nil {
		log.Errorf(err.Error())
		debug.PrintStack()
		return
	}
}

func (s *SpiderSync) AfterRemoveDownCreate() {
	if model.IsMaster() {
		s.CheckIsScrapy()
	}
}

func (s *SpiderSync) RemoveDownCreate(md5 string) {
	s.RemoveSpiderFile()
	s.Download()
	s.CreateMd5File(md5)
	s.AfterRemoveDownCreate()
}

// 获得下载锁的key
func (s *SpiderSync) GetLockDownloadKey(spiderId string) string {
	//node, _ := model.GetCurrentNode()
	node := local_node.CurrentNode()

	return node.Id.Hex() + "#" + spiderId
}

// 删除本地文件
func (s *SpiderSync) RemoveSpiderFile() {
	path := filepath.Join(
		viper.GetString("spider.path"),
		s.Spider.Name,
	)
	//爬虫文件有变化，先删除本地文件
	if err := os.RemoveAll(path); err != nil {
		log.Errorf("remove spider files error: %s, path: %s", err.Error(), path)
		debug.PrintStack()
	}
}

// 检测是否已经下载中
func (s *SpiderSync) CheckDownLoading(spiderId string, fileId string) (bool, string) {
	key := s.GetLockDownloadKey(spiderId)
	key2, err := database.RedisClient.HGet("spider", key)
	if err != nil {
		return false, key2
	}
	if key2 == "" {
		return false, key2
	}
	return true, key2
}

// 下载爬虫
func (s *SpiderSync) Download() {
	spiderId := s.Spider.Id.Hex()
	fileId := s.Spider.FileId.Hex()
	isDownloading, key := s.CheckDownLoading(spiderId, fileId)
	if isDownloading {
		log.Infof(fmt.Sprintf("spider is already being downloaded, spider id: %s", s.Spider.Id.Hex()))
		return
	} else {
		_ = database.RedisClient.HSet("spider", key, key)
	}

	session, gf := database.GetGridFs("files")
	defer session.Close()

	f, err := gf.OpenId(bson.ObjectIdHex(fileId))
	if err != nil {
		log.Errorf("open file id: " + fileId + ", spider id:" + spiderId + ", error: " + err.Error())
		debug.PrintStack()
		return
	}
	defer utils.Close(f)

	// 生成唯一ID
	randomId := uuid.NewV4()
	tmpPath := viper.GetString("other.tmppath")
	if !utils.Exists(tmpPath) {
		if err := os.MkdirAll(tmpPath, 0777); err != nil {
			log.Errorf("mkdir other.tmppath error: %v", err.Error())
			return
		}
	}
	// 创建临时文件
	tmpFilePath := filepath.Join(tmpPath, randomId.String()+".zip")
	tmpFile := utils.OpenFile(tmpFilePath)

	// 将该文件写入临时文件
	if _, err := io.Copy(tmpFile, f); err != nil {
		log.Errorf("copy file error: %s, file_id: %s", err.Error(), f.Id())
		debug.PrintStack()
		return
	}

	// 解压缩临时文件到目标文件夹
	dstPath := filepath.Join(
		viper.GetString("spider.path"),
		s.Spider.Name,
	)
	if err := utils.DeCompress(tmpFile, dstPath); err != nil {
		log.Errorf(err.Error())
		debug.PrintStack()
		return
	}

	//递归修改目标文件夹权限
	// 解决scrapy.setting中开启LOG_ENABLED 和 LOG_FILE时不能创建log文件的问题
	cmd := exec.Command("chmod", "-R", "777", dstPath)
	if err := cmd.Run(); err != nil {
		log.Errorf(err.Error())
		debug.PrintStack()
		return
	}

	// 关闭临时文件
	if err := tmpFile.Close(); err != nil {
		log.Errorf(err.Error())
		debug.PrintStack()
		return
	}

	// 删除临时文件
	if err := os.Remove(tmpFilePath); err != nil {
		log.Errorf(err.Error())
		debug.PrintStack()
		return
	}

	_ = database.RedisClient.HDel("spider", key)
}

// locks for dependency installation
var installLockMap sync.Map

// install dependencies
func (s *SpiderSync) InstallDeps() {
	langs := utils.GetLangList()
	for _, l := range langs {
		// no dep file name is found, skip
		if l.DepFileName == "" {
			continue
		}

		// being locked, i.e. installation is running, skip
		key := s.Spider.Name + "|" + l.Name
		_, locked := installLockMap.Load(key)
		if locked {
			continue
		}

		// no dep file found, skip
		if !utils.Exists(path.Join(s.Spider.Src, l.DepFileName)) {
			continue
		}

		// no dep install executable found, skip
		if !utils.Exists(l.DepExecutablePath) {
			continue
		}

		// lock
		installLockMap.Store(key, true)

		// command to install dependencies
		cmd := exec.Command(l.DepExecutablePath, strings.Split(l.InstallDepArgs, " ")...)

		// working directory
		cmd.Dir = s.Spider.Src

		// compatibility with node.js
		if l.ExecutableName == constants.Nodejs {
			deps, err := utils.GetPackageJsonDeps(path.Join(s.Spider.Src, l.DepFileName))
			if err != nil {
				continue
			}
			cmd = exec.Command(l.DepExecutablePath, strings.Split(l.InstallDepArgs+" "+strings.Join(deps, " "), " ")...)
		}

		// start executing command
		output, err := cmd.Output()
		if err != nil {
			log.Errorf("install dep error: " + err.Error())
			log.Errorf(string(output))
			debug.PrintStack()
		}

		// unlock
		installLockMap.Delete(key)
	}
}
