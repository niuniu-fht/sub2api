package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// SettingKeyChannelStatusAnnouncement 渠道状态公告（settings 表单键）。
// 只维护一条：新公告整体覆盖旧公告，无历史。
const SettingKeyChannelStatusAnnouncement = "channel_status_announcement"

// ChannelStatusAnnouncementMaxLen 公告内容长度上限（字符），防滥用。
const ChannelStatusAnnouncementMaxLen = 2000

// ChannelStatusAnnouncement 渠道状态页公告（单条，覆盖式）。
type ChannelStatusAnnouncement struct {
	Content     string `json:"content"`
	UpdatedAtMs int64  `json:"updated_at_ms"`
}

// GetChannelStatusAnnouncement 读取当前公告；未配置时返回零值。
func (s *SettingService) GetChannelStatusAnnouncement(ctx context.Context) (ChannelStatusAnnouncement, error) {
	zero := ChannelStatusAnnouncement{}
	if s == nil || s.settingRepo == nil {
		return zero, nil
	}
	setting, err := s.settingRepo.Get(ctx, SettingKeyChannelStatusAnnouncement)
	if err != nil {
		if err == ErrSettingNotFound {
			return zero, nil
		}
		return zero, err
	}
	var ann ChannelStatusAnnouncement
	if err := json.Unmarshal([]byte(setting.Value), &ann); err != nil {
		// 脏数据视为无公告，下次保存即修复。
		return zero, nil
	}
	return ann, nil
}

// SetChannelStatusAnnouncement 覆盖保存公告：内容为空即清除。
// updated_at_ms 用保存时刻，前端以它判断「新公告」并重新弹出已关闭的悬浮条。
func (s *SettingService) SetChannelStatusAnnouncement(ctx context.Context, content string) (ChannelStatusAnnouncement, error) {
	content = strings.TrimSpace(content)
	if len([]rune(content)) > ChannelStatusAnnouncementMaxLen {
		return ChannelStatusAnnouncement{}, infraerrors.Newf(400, "INVALID_ANNOUNCEMENT", "announcement content exceeds %d characters", ChannelStatusAnnouncementMaxLen)
	}
	ann := ChannelStatusAnnouncement{}
	if s == nil || s.settingRepo == nil {
		return ann, nil
	}
	if content == "" {
		if err := s.settingRepo.Set(ctx, SettingKeyChannelStatusAnnouncement, "{}"); err != nil {
			return ann, err
		}
		return ann, nil
	}
	ann = ChannelStatusAnnouncement{Content: content, UpdatedAtMs: time.Now().UnixMilli()}
	payload, err := json.Marshal(ann)
	if err != nil {
		return ann, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyChannelStatusAnnouncement, string(payload)); err != nil {
		return ChannelStatusAnnouncement{}, err
	}
	return ann, nil
}
