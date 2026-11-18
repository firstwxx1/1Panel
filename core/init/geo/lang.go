package geo

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/cmd"
	fileUtils "github.com/1Panel-dev/1Panel/core/utils/files"
)

func Init() {
	go initLang()
}

func initLang() {
	geoPath := path.Join(global.CONF.Base.InstallDir, "1panel/geo/GeoIP.mmdb")
	isLangExist := fileUtils.Stat("/usr/local/bin/lang/zh.sh")
	isGeoExist := fileUtils.Stat(geoPath)
	if isLangExist && isGeoExist {
		return
	}
	upgradePath := path.Join(global.CONF.Base.InstallDir, "1panel/tmp/upgrade")
	tmpPath, err := loadRestorePath(upgradePath)
	upgradeDir := path.Join(upgradePath, tmpPath, "downloads")
	if err != nil || len(tmpPath) == 0 || !fileUtils.Stat(upgradeDir) {
		if !isLangExist {
			downloadLangFromRemote()
		}
		if !isGeoExist {
			downloadGeoFromRemote(geoPath)
		}
		return
	}

	files, _ := os.ReadDir(upgradeDir)
	if len(files) == 0 {
		tmpPath = "no such file"
	} else {
		for _, item := range files {
			if item.IsDir() && strings.HasPrefix(item.Name(), "1panel-") {
				tmpPath = path.Join(upgradePath, tmpPath, "downloads", item.Name())
				break
			}
		}
	}
	if tmpPath == "no such file" || !fileUtils.Stat(tmpPath) {
		if !isLangExist {
			downloadLangFromRemote()
		}
		if !isGeoExist {
			downloadGeoFromRemote(geoPath)
		}
		return
	}
	if !isLangExist {
		if !fileUtils.Stat(path.Join(tmpPath, "lang")) {
			downloadLangFromRemote()
			return
		}
		if err := cmd.NewCommandMgr().Run("cp", "-r", path.Join(tmpPath, "lang"), "/usr/local/bin/"); err != nil {
			global.LOG.Errorf("load lang from package failed, %v", err)
			return
		}
		global.LOG.Info("init lang successful")
	}
	if !isGeoExist {
		if !fileUtils.Stat(path.Join(tmpPath, "GeoIP.mmdb")) {
			downloadGeoFromRemote(geoPath)
			return
		}
		if err := os.MkdirAll(path.Dir(geoPath), os.ModePerm); err != nil {
			global.LOG.Errorf("mkdir geo ip dir failed, %v", err)
			return
		}
		if err := cmd.NewCommandMgr().Run("cp", path.Join(tmpPath, "GeoIP.mmdb"), path.Dir(geoPath)+"/"); err != nil {
			global.LOG.Errorf("load geo ip from package failed, %v", err)
			return
		}
		global.LOG.Info("init geo ip successful")
	}
}

func loadRestorePath(upgradeDir string) (string, error) {
	if _, err := os.Stat(upgradeDir); err != nil && os.IsNotExist(err) {
		return "no such file", nil
	}
	files, err := os.ReadDir(upgradeDir)
	if err != nil {
		return "", err
	}
	var folders []string
	for _, file := range files {
		if file.IsDir() {
			folders = append(folders, file.Name())
		}
	}
	if len(folders) == 0 {
		return "no such file", nil
	}
	sort.Slice(folders, func(i, j int) bool {
		return folders[i] > folders[j]
	})
	return folders[0], nil
}

func copyGeoFromLocalOpenResty(targetPath string) bool {
	patterns := []string{
		filepath.Join(global.CONF.Base.InstallDir, "1panel/apps/openresty/*/1pwaf/data/geo/GeoIP.mmdb"),
		filepath.Join(global.CONF.Base.InstallDir, "1panel/apps/local/openresty/*/1pwaf/data/geo/GeoIP.mmdb"),
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, src := range matches {
			st, err := os.Stat(src)
			if err != nil || st.IsDir() || st.Size() <= 100*1024 {
				continue
			}
			if err := os.MkdirAll(path.Dir(targetPath), 0o755); err != nil {
				continue
			}
			in, err := os.Open(src)
			if err != nil {
				continue
			}
			tmp := targetPath + ".tmp"
			out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				_ = in.Close()
				continue
			}
			_, cpErr := io.Copy(out, in)
			_ = in.Close()
			clErr := out.Close()
			if cpErr != nil || clErr != nil {
				_ = os.Remove(tmp)
				continue
			}
			if err := os.Rename(tmp, targetPath); err == nil {
				global.LOG.Info("copied GeoIP.mmdb from local openresty 1pwaf")
				return true
			}
			_ = os.Remove(tmp)
		}
	}
	return false
}

func downloadLangFromRemote() {
	if _, err := os.Stat("/usr/local/bin/1pctl"); err != nil {
		return
	}
	pathItem := fmt.Sprintf("%s/language/lang.tar.gz", global.ResourceURL())
	if err := fileUtils.DownloadFile(pathItem, "/usr/local/bin/lang.tar.gz"); err != nil {
		_ = os.Remove("/usr/local/bin/lang.tar.gz")
		global.LOG.Debugf("download lang.tar.gz skipped: %v", err)
		return
	}
	if !fileUtils.Stat("/usr/local/bin/lang.tar.gz") {
		return
	}
	if err := cmd.NewCommandMgr().Run("tar", "zxvfC", "/usr/local/bin/lang.tar.gz", "/usr/local/bin/"); err != nil {
		_ = os.Remove("/usr/local/bin/lang.tar.gz")
		global.LOG.Debugf("decompress lang.tar.gz skipped: %v", err)
		return
	}
	_ = os.Remove("/usr/local/bin/lang.tar.gz")
	global.LOG.Info("download lang successful")
}

func downloadGeoFromRemote(targetPath string) {
	_ = os.MkdirAll(path.Dir(targetPath), os.ModePerm)
	if copyGeoFromLocalOpenResty(targetPath) {
		return
	}
	pathItem := fmt.Sprintf("%s/geo/GeoIP.mmdb", global.ResourceURL())
	if err := fileUtils.DownloadFile(pathItem, targetPath); err != nil {
		_ = os.Remove(targetPath)
		if copyGeoFromLocalOpenResty(targetPath) {
			return
		}
		global.LOG.Warnf("download geo ip failed, err: %v", err)
		return
	}
	global.LOG.Info("download geo ip successful")
}
