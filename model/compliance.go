// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package model

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const (
	ComplianceStatusCreated  = "created"
	ComplianceStatusRunning  = "running"
	ComplianceStatusFinished = "finished"
	ComplianceStatusFailed   = "failed"
	ComplianceStatusRemoved  = "removed"

	ComplianceTypeDaily = "daily"
	ComplianceTypeAdhoc = "adhoc"
)

type Compliance struct {
	Id       string `json:"id"`
	CreateAt int64  `json:"create_at"`
	UserId   string `json:"user_id"`
	Status   string `json:"status"`
	Count    int    `json:"count"`
	Desc     string `json:"desc"`
	Type     string `json:"type"`
	StartAt  int64  `json:"start_at"`
	EndAt    int64  `json:"end_at"`
	Keywords string `json:"keywords"`
	Emails   string `json:"emails"`
}

type Compliances []Compliance

// ComplianceExportCursor is used for paginated iteration of posts
// for compliance export.
// We need to keep track of the last post ID in addition to the last post
// CreateAt to break ties when two posts have the same CreateAt.
type ComplianceExportCursor struct {
	LastChannelsQueryPostCreateAt       int64
	LastChannelsQueryPostID             string
	ChannelsQueryCompleted              bool
	LastDirectMessagesQueryPostCreateAt int64
	LastDirectMessagesQueryPostID       string
	DirectMessagesQueryCompleted        bool
}

func (c *Compliance) PreSave() {
	if c.Id == "" {
		c.Id = NewId()
	}

	if c.Status == "" {
		c.Status = ComplianceStatusCreated
	}

	c.Count = 0
	c.Emails = NormalizeEmail(c.Emails)
	c.Keywords = strings.ToLower(c.Keywords)

	c.CreateAt = GetMillis()
}

func (c *Compliance) DeepCopy() *Compliance {
	copy := *c
	return &copy
}

func (c *Compliance) JobName() string {
	jobName := c.Type
	if c.Type == ComplianceTypeDaily {
		jobName += "-" + c.Desc
	}

	jobName += "-" + c.Id

	return jobName
}

func (c *Compliance) IsValid() *AppError {

	if !IsValidId(c.Id) {
		return NewAppError("Compliance.IsValid", "model.compliance.is_valid.id.app_error", nil, "", http.StatusBadRequest)
	}

	if c.CreateAt == 0 {
		return NewAppError("Compliance.IsValid", "model.compliance.is_valid.create_at.app_error", nil, "", http.StatusBadRequest)
	}

	if len(c.Desc) > 512 || c.Desc == "" {
		return NewAppError("Compliance.IsValid", "model.compliance.is_valid.desc.app_error", nil, "", http.StatusBadRequest)
	}

	if c.StartAt == 0 {
		return NewAppError("Compliance.IsValid", "model.compliance.is_valid.start_at.app_error", nil, "", http.StatusBadRequest)
	}

	if c.EndAt == 0 {
		return NewAppError("Compliance.IsValid", "model.compliance.is_valid.end_at.app_error", nil, "", http.StatusBadRequest)
	}

	if c.EndAt <= c.StartAt {
		return NewAppError("Compliance.IsValid", "model.compliance.is_valid.start_end_at.app_error", nil, "", http.StatusBadRequest)
	}

	return nil
}

func ComplianceFromJson(data io.Reader) *Compliance {
	var c *Compliance
	json.NewDecoder(data).Decode(&c)
	return c
}

func CompliancesFromJson(data io.Reader) Compliances {
	var o Compliances
	json.NewDecoder(data).Decode(&o)
	return o
}
