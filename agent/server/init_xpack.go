//go:build xpack || enterprise

package server

import (
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/app/service"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/xpack"
	xpackHelper "github.com/1Panel-dev/1Panel/agent/utils/xpack/helper"
	"github.com/1Panel-dev/1Panel/agent/xpack/waf"
)

var wafMaintOnce sync.Once

func InitOthers() {
	xpackHelper.StartClamJobHook = func(clamID uint) {
		_ = service.NewIClamService().HandleOnce(clamID)
	}
	if global.DB != nil && global.Cron != nil {
		var clams []model.Clam
		if err := global.DB.Where("status = ? AND spec != ?", constant.StatusEnable, "").Find(&clams).Error; err == nil {
			for i := range clams {
				entryID, err := xpack.MultiNodeProvider.StartClam(&clams[i], false)
				if err == nil {
					_ = global.DB.Model(&model.Clam{}).Where("id = ?", clams[i].ID).Update("entry_id", entryID).Error
				} else if global.LOG != nil {
					global.LOG.Errorf("start clam job %s failed, err: %v", clams[i].Name, err)
				}
			}
		}
	}
	wafMaintOnce.Do(func() {
		go runWafMaintenanceLoop()
	})
}

func runWafMaintenanceLoop() {
	time.Sleep(10 * time.Minute)
	for {
		func() {
			defer func() {
				if r := recover(); r != nil && global.LOG != nil {
					global.LOG.Errorf("waf maintenance panic recovered: %v", r)
				}
			}()
			app, err := repo.NewIAppRepo().GetFirst(repo.NewIAppRepo().WithKey(constant.AppOpenresty))
			if err != nil || app.ID == 0 {
				return
			}
			install, err := repo.NewIAppInstallRepo().GetFirst(repo.NewIAppInstallRepo().WithAppId(app.ID))
			if err != nil || install.ID == 0 {
				return
			}
			s := &waf.Store{
				OpenrestyPath: install.GetPath(),
				Container:     install.ContainerName,
			}
			if err := s.RunMaintenance(); err != nil && global.LOG != nil {
				global.LOG.Warnf("waf daily log maintenance failed: %v", err)
			}
		}()
		time.Sleep(24 * time.Hour)
	}
}
