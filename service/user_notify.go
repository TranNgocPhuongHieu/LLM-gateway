package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func NotifyRootUser(t string, subject string, content string) {
	user := model.GetRootUser().ToBaseUser()
	err := NotifyUser(user.Id, user.Email, user.GetSetting(), dto.NewNotify(t, subject, content, nil))
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to notify root user: %s", err.Error()))
	}
}

func NotifyUpstreamModelUpdateWatchers(subject string, content string) {
	var users []model.User
	if err := model.DB.
		Select("id", "email", "role", "status", "setting").
		Where("status = ? AND role >= ?", common.UserStatusEnabled, common.RoleAdminUser).
		Find(&users).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to query upstream update notification users: %s", err.Error()))
		return
	}

	notification := dto.NewNotify(dto.NotifyTypeChannelUpdate, subject, content, nil)
	sentCount := 0
	for _, user := range users {
		userSetting := user.GetSetting()
		if !userSetting.UpstreamModelUpdateNotifyEnabled {
			continue
		}
		if err := NotifyUser(user.Id, user.Email, userSetting, notification); err != nil {
			common.SysLog(fmt.Sprintf("failed to notify user %d for upstream model update: %s", user.Id, err.Error()))
			continue
		}
		sentCount++
	}
	common.SysLog(fmt.Sprintf("upstream model update notifications sent: %d", sentCount))
}

// NotifyUser supports email only; retired delivery preferences must be changed explicitly.
func NotifyUser(userId int, userEmail string, userSetting dto.UserSetting, data dto.Notify) error {
	if userSetting.NotifyType != "" && userSetting.NotifyType != dto.NotifyTypeEmail {
		return fmt.Errorf("unsupported notification method: %s", userSetting.NotifyType)
	}
	canSend, err := CheckNotificationLimit(userId, data.Type)
	if err != nil {
		return err
	}
	if !canSend {
		return fmt.Errorf("notification limit exceeded for user %d", userId)
	}
	email := userSetting.NotificationEmail
	if email == "" {
		email = userEmail
	}
	if email == "" {
		common.SysLog(fmt.Sprintf("user %d has no email, skip sending email", userId))
		return nil
	}
	return sendEmailNotify(email, data)
}

func sendEmailNotify(userEmail string, data dto.Notify) error {
	// make email content
	content := data.Content
	// 处理占位符
	for _, value := range data.Values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
	}
	return common.SendEmail(data.Title, userEmail, content)
}
