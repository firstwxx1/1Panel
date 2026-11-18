package helper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	alertUtil "github.com/1Panel-dev/1Panel/agent/utils/alert"
	"github.com/1Panel-dev/1Panel/agent/utils/xpack/providers"
)

type alertHelper struct{}

var (
	_ providers.CustomWebhookTester           = (*alertHelper)(nil)
	_ providers.CustomWebhookDeliveryProvider = (*alertHelper)(nil)
)

var loadCommunityCustomWebhookContext = func() (*http.Transport, *dto.AgentInfo) {
	multiNode := &multiNodeHelper{}
	agentInfo, _ := multiNode.GetAgentInfo()
	return multiNode.LoadRequestTransport(), agentInfo
}

func NewIAlertProvider() providers.AlertProvider {
	return &alertHelper{}
}

func (a *alertHelper) CreateTaskScanSMSAlertLog(alert dto.AlertDTO, alertType string, create dto.AlertLogCreate, pushAlert dto.PushAlert, config model.AlertConfig, method string) error {
	return nil
}

func (a *alertHelper) CreateSMSAlertLog(alertType string, info dto.AlertDTO, create dto.AlertLogCreate, project string, params []dto.Param, config model.AlertConfig, method string) error {
	return nil
}

func (a *alertHelper) CreateTaskScanWebhookAlertLog(alert dto.AlertDTO, alertType string, create dto.AlertLogCreate, pushAlert dto.PushAlert, config model.AlertConfig, transport *http.Transport, agentInfo *dto.AgentInfo) error {
	if config.Type == constant.Custom {
		return alertUtil.CreateTaskScanCustomWebhookAlertLog(alert, alertType, create, pushAlert, config, transport, agentInfo)
	}
	if config.Type == constant.WeCom || config.Type == constant.DingTalk || config.Type == constant.FeiShu {
		params := alertUtil.CreateTaskAlertParams(pushAlert)
		alertInfo := alert
		if alertType != "" {
			alertInfo.Type = alertType
		}
		create.Type = alertUtil.GetCronJobType(alertInfo.Type)
		create.AlertDetail = alertUtil.ProcessAlertDetail(alertInfo, pushAlert.TaskName, params, config.Type)
		create.AlertRule = alertUtil.ProcessAlertRule(alert)
		return a.CreateWebhookAlertLog(alertType, alert, create, pushAlert.TaskName, params, config, transport, agentInfo)
	}
	return nil
}

func (a *alertHelper) CreateWebhookAlertLog(alertType string, info dto.AlertDTO, create dto.AlertLogCreate, project string, params []dto.Param, config model.AlertConfig, transport *http.Transport, agentInfo *dto.AgentInfo) error {
	if config.Type == constant.Custom {
		return alertUtil.CreateCustomWebhookAlertLog(alertType, info, create, project, params, config, transport, agentInfo)
	}
	if config.Type != constant.WeCom && config.Type != constant.DingTalk && config.Type != constant.FeiShu {
		return nil
	}

	alertInfo := info
	if alertType != "" {
		alertInfo.Type = alertType
	}
	if create.Type == "" {
		create.Type = alertUtil.GetCronJobType(alertInfo.Type)
	}
	if create.AlertRule == "" {
		create.AlertRule = alertUtil.ProcessAlertRule(info)
	}
	if create.AlertDetail == "" {
		create.AlertDetail = alertUtil.ProcessAlertDetail(alertInfo, project, params, config.Type)
	}

	var alertLog model.AlertLog
	var webhookInfo dto.AlertWebhookConfig
	if err := json.Unmarshal([]byte(config.Config), &webhookInfo); err != nil {
		return err
	}
	webhookURL := strings.TrimSpace(webhookInfo.Url)
	if webhookURL == "" {
		create.Status = constant.AlertError
		create.Message = "webhook url is required"
		return alertUtil.SaveAlertLog(create, &alertLog)
	}

	title := i18n.GetMsgByKey("PanelAlertTitle")
	content := alertUtil.GetAlertLogContent(create, alertInfo, params, agentInfo)
	if strings.TrimSpace(content) == "" {
		content = alertInfo.Title
	}
	fullText := title + "\n" + content
	if strings.TrimSpace(title) == "" {
		fullText = content
	}

	var payload map[string]interface{}
	if config.Type == constant.FeiShu {
		payload = map[string]interface{}{
			"msg_type": "text",
			"content": map[string]string{
				"text": fullText,
			},
		}
	} else {
		payload = map[string]interface{}{
			"msgtype": "text",
			"text": map[string]string{
				"content": fullText,
			},
		}
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		create.Status = constant.AlertError
		create.Message = err.Error()
		return alertUtil.SaveAlertLog(create, &alertLog)
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(bodyBytes))
	if err != nil {
		create.Status = constant.AlertError
		create.Message = err.Error()
		return alertUtil.SaveAlertLog(create, &alertLog)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		create.Status = constant.AlertError
		create.Message = err.Error()
		return alertUtil.SaveAlertLog(create, &alertLog)
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(respBytes))
		if msg == "" {
			msg = fmt.Sprintf("webhook http status %d", resp.StatusCode)
		}
		create.Status = constant.AlertError
		create.Message = msg
		return alertUtil.SaveAlertLog(create, &alertLog)
	}

	var apiResp struct {
		ErrCode *int   `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Code    *int   `json:"code"`
		Msg     string `json:"msg"`
	}
	if len(respBytes) > 0 && json.Unmarshal(respBytes, &apiResp) == nil {
		if apiResp.Code != nil && *apiResp.Code != 0 {
			msg := strings.TrimSpace(apiResp.Msg)
			if msg == "" {
				msg = fmt.Sprintf("webhook error code %d", *apiResp.Code)
			}
			create.Status = constant.AlertError
			create.Message = msg
			return alertUtil.SaveAlertLog(create, &alertLog)
		}
		if apiResp.ErrCode != nil && *apiResp.ErrCode != 0 {
			msg := strings.TrimSpace(apiResp.ErrMsg)
			if msg == "" {
				msg = fmt.Sprintf("webhook error code %d", *apiResp.ErrCode)
			}
			create.Status = constant.AlertError
			create.Message = msg
			return alertUtil.SaveAlertLog(create, &alertLog)
		}
	}

	create.Status = constant.AlertSuccess
	create.Message = ""
	return alertUtil.SaveAlertLog(create, &alertLog)
}

func (a *alertHelper) CreateCustomWebhookAlertLog(alertType string, info dto.AlertDTO, create dto.AlertLogCreate, project string, params []dto.Param, config model.AlertConfig, transport *http.Transport, agentInfo *dto.AgentInfo, _ dto.AlertTaskMetadata) (providers.DeliveryResult, error) {
	err := alertUtil.CreateCustomWebhookAlertLog(alertType, info, create, project, params, config, transport, agentInfo)
	return providers.DeliveryResult{}, err
}

func (a *alertHelper) CreateTaskScanCustomWebhookAlertLog(alert dto.AlertDTO, alertType string, create dto.AlertLogCreate, pushAlert dto.PushAlert, config model.AlertConfig, transport *http.Transport, agentInfo *dto.AgentInfo, _ dto.AlertTaskMetadata) (providers.DeliveryResult, error) {
	err := alertUtil.CreateTaskScanCustomWebhookAlertLog(alert, alertType, create, pushAlert, config, transport, agentInfo)
	return providers.DeliveryResult{}, err
}

func (a *alertHelper) TestCustomWebhook(config dto.AlertCustomWebhookResolvedConfig, lang string) (dto.AlertConfigTestResult, error) {
	transport, agentInfo := loadCommunityCustomWebhookContext()
	return alertUtil.TestCustomWebhook(config, transport, agentInfo, lang)
}

func (a *alertHelper) GetLicenseErrorAlert() (uint, error) {
	return 0, nil
}

func (a *alertHelper) GetNodeErrorAlert() (uint, error) {
	if global.IsMaster && global.CoreDB != nil {
		var cnt int64
		_ = global.CoreDB.Table("nodes").Where("kind = ? AND status IN ?", "node", []string{"Unhealthy", "Failed"}).Count(&cnt).Error
		return uint(cnt), nil
	}
	return 0, nil
}
