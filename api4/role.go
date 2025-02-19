// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"encoding/json"
	"net/http"

	"github.com/mattermost/mattermost-server/v6/audit"
	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

var notAllowedPermissions = []string{
	model.PermissionSysconsoleWriteUserManagementSystemRoles.Id,
	model.PermissionSysconsoleReadUserManagementSystemRoles.Id,
	model.PermissionManageRoles.Id,
}

func (api *API) InitRole() {
	api.BaseRoutes.Roles.Handle("/{role_id:[A-Za-z0-9]+}", api.ApiSessionRequiredTrustRequester(getRole)).Methods("GET")
	api.BaseRoutes.Roles.Handle("/name/{role_name:[a-z0-9_]+}", api.ApiSessionRequiredTrustRequester(getRoleByName)).Methods("GET")
	api.BaseRoutes.Roles.Handle("/names", api.ApiSessionRequiredTrustRequester(getRolesByNames)).Methods("POST")
	api.BaseRoutes.Roles.Handle("/{role_id:[A-Za-z0-9]+}/patch", api.ApiSessionRequired(patchRole)).Methods("PUT")
}

func getRole(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireRoleId()
	if c.Err != nil {
		return
	}

	role, err := c.App.GetRole(c.Params.RoleId)
	if err != nil {
		c.Err = err
		return
	}

	if err := json.NewEncoder(w).Encode(role); err != nil {
		mlog.Warn("Error while writing response", mlog.Err(err))
	}
}

func getRoleByName(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireRoleName()
	if c.Err != nil {
		return
	}

	role, err := c.App.GetRoleByName(r.Context(), c.Params.RoleName)
	if err != nil {
		c.Err = err
		return
	}

	if err := json.NewEncoder(w).Encode(role); err != nil {
		mlog.Warn("Error while writing response", mlog.Err(err))
	}
}

func getRolesByNames(c *Context, w http.ResponseWriter, r *http.Request) {
	rolenames := model.ArrayFromJson(r.Body)

	if len(rolenames) == 0 {
		c.SetInvalidParam("rolenames")
		return
	}

	cleanedRoleNames, valid := model.CleanRoleNames(rolenames)
	if !valid {
		c.SetInvalidParam("rolename")
		return
	}

	roles, err := c.App.GetRolesByNames(cleanedRoleNames)
	if err != nil {
		c.Err = err
		return
	}

	w.Write([]byte(model.RoleListToJson(roles)))
}

func patchRole(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireRoleId()
	if c.Err != nil {
		return
	}

	patch := model.RolePatchFromJson(r.Body)
	if patch == nil {
		c.SetInvalidParam("role")
		return
	}

	auditRec := c.MakeAuditRecord("patchRole", audit.Fail)
	defer c.LogAuditRec(auditRec)

	oldRole, err := c.App.GetRole(c.Params.RoleId)
	if err != nil {
		c.Err = err
		return
	}
	auditRec.AddMeta("role", oldRole)

	// manage_system permission is required to patch system_admin
	requiredPermission := model.PermissionSysconsoleWriteUserManagementPermissions
	specialProtectedSystemRoles := append(model.NewSystemRoleIDs, model.SystemAdminRoleId)
	for _, roleID := range specialProtectedSystemRoles {
		if oldRole.Name == roleID {
			requiredPermission = model.PermissionManageSystem
		}
	}
	if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), requiredPermission) {
		c.SetPermissionError(requiredPermission)
		return
	}

	isGuest := oldRole.Name == model.SystemGuestRoleId || oldRole.Name == model.TeamGuestRoleId || oldRole.Name == model.ChannelGuestRoleId
	if c.App.Srv().License() == nil && patch.Permissions != nil {
		if isGuest {
			c.Err = model.NewAppError("Api4.PatchRoles", "api.roles.patch_roles.license.error", nil, "", http.StatusNotImplemented)
			return
		}
	}

	// Licensed instances can not change permissions in the blacklist set.
	if patch.Permissions != nil {
		deltaPermissions := model.PermissionsChangedByPatch(oldRole, patch)

		for _, permission := range deltaPermissions {
			notAllowed := false
			for _, notAllowedPermission := range notAllowedPermissions {
				if permission == notAllowedPermission {
					notAllowed = true
				}
			}

			if notAllowed {
				c.Err = model.NewAppError("Api4.PatchRoles", "api.roles.patch_roles.not_allowed_permission.error", nil, "Cannot add or remove permission: "+permission, http.StatusNotImplemented)
				return
			}
		}

		*patch.Permissions = model.RemoveDuplicateStrings(*patch.Permissions)
	}

	if c.App.Srv().License() != nil && isGuest && !*c.App.Srv().License().Features.GuestAccountsPermissions {
		c.Err = model.NewAppError("Api4.PatchRoles", "api.roles.patch_roles.license.error", nil, "", http.StatusNotImplemented)
		return
	}

	if oldRole.Name == model.TeamAdminRoleId || oldRole.Name == model.ChannelAdminRoleId || oldRole.Name == model.SystemUserRoleId || oldRole.Name == model.TeamUserRoleId || oldRole.Name == model.ChannelUserRoleId || oldRole.Name == model.SystemGuestRoleId || oldRole.Name == model.TeamGuestRoleId || oldRole.Name == model.ChannelGuestRoleId {
		if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionSysconsoleWriteUserManagementPermissions) {
			c.SetPermissionError(model.PermissionSysconsoleWriteUserManagementPermissions)
			return
		}
	} else {
		if !c.App.SessionHasPermissionTo(*c.AppContext.Session(), model.PermissionSysconsoleWriteUserManagementSystemRoles) {
			c.SetPermissionError(model.PermissionSysconsoleWriteUserManagementSystemRoles)
			return
		}
	}

	role, err := c.App.PatchRole(oldRole, patch)
	if err != nil {
		c.Err = err
		return
	}

	auditRec.Success()
	auditRec.AddMeta("patch", role)
	c.LogAudit("")

	if err := json.NewEncoder(w).Encode(role); err != nil {
		mlog.Warn("Error while writing response", mlog.Err(err))
	}
}
