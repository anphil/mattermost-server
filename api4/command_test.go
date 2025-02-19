// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

func TestCreateCommand(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	Client := th.Client
	LocalClient := th.LocalClient

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })

	newCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    th.BasicTeam.Id,
		URL:       "http://nowhere.com",
		Method:    model.CommandMethodPost,
		Trigger:   "trigger"}

	_, resp := Client.CreateCommand(newCmd)
	CheckForbiddenStatus(t, resp)

	createdCmd, resp := th.SystemAdminClient.CreateCommand(newCmd)
	CheckNoError(t, resp)
	CheckCreatedStatus(t, resp)
	require.Equal(t, th.SystemAdminUser.Id, createdCmd.CreatorId, "user ids didn't match")
	require.Equal(t, th.BasicTeam.Id, createdCmd.TeamId, "team ids didn't match")

	_, resp = th.SystemAdminClient.CreateCommand(newCmd)
	CheckBadRequestStatus(t, resp)
	CheckErrorMessage(t, resp, "api.command.duplicate_trigger.app_error")

	newCmd.Trigger = "Local"
	localCreatedCmd, resp := LocalClient.CreateCommand(newCmd)
	CheckNoError(t, resp)
	CheckCreatedStatus(t, resp)
	require.Equal(t, th.BasicUser.Id, localCreatedCmd.CreatorId, "local client: user ids didn't match")
	require.Equal(t, th.BasicTeam.Id, localCreatedCmd.TeamId, "local client: team ids didn't match")

	newCmd.Method = "Wrong"
	newCmd.Trigger = "testcommand"
	_, resp = th.SystemAdminClient.CreateCommand(newCmd)
	CheckBadRequestStatus(t, resp)
	CheckErrorMessage(t, resp, "model.command.is_valid.method.app_error")

	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = false })
	newCmd.Method = "P"
	newCmd.Trigger = "testcommand"
	_, resp = th.SystemAdminClient.CreateCommand(newCmd)
	CheckNotImplementedStatus(t, resp)
	CheckErrorMessage(t, resp, "api.command.disabled.app_error")

	// Confirm that local clients can't override disable command setting
	newCmd.Trigger = "LocalOverride"
	_, resp = LocalClient.CreateCommand(newCmd)
	CheckErrorMessage(t, resp, "api.command.disabled.app_error")
}

func TestUpdateCommand(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	user := th.SystemAdminUser
	team := th.BasicTeam

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })

	cmd1 := &model.Command{
		CreatorId: user.Id,
		TeamId:    team.Id,
		URL:       "http://nowhere.com",
		Method:    model.CommandMethodPost,
		Trigger:   "trigger1",
	}

	cmd1, _ = th.App.CreateCommand(cmd1)

	cmd2 := &model.Command{
		CreatorId: GenerateTestId(),
		TeamId:    team.Id,
		URL:       "http://nowhere.com/change",
		Method:    model.CommandMethodGet,
		Trigger:   "trigger2",
		Id:        cmd1.Id,
		Token:     "tokenchange",
	}

	th.TestForSystemAdminAndLocal(t, func(t *testing.T, client *model.Client4) {
		rcmd, resp := client.UpdateCommand(cmd2)
		CheckNoError(t, resp)

		require.Equal(t, cmd2.Trigger, rcmd.Trigger, "Trigger should have updated")

		require.Equal(t, cmd2.Method, rcmd.Method, "Method should have updated")

		require.Equal(t, cmd2.URL, rcmd.URL, "URL should have updated")

		require.Equal(t, cmd1.CreatorId, rcmd.CreatorId, "CreatorId should have not updated")

		require.Equal(t, cmd1.Token, rcmd.Token, "Token should have not updated")

		cmd2.Id = GenerateTestId()

		rcmd, resp = client.UpdateCommand(cmd2)
		CheckNotFoundStatus(t, resp)

		require.Nil(t, rcmd, "should be empty")

		cmd2.Id = "junk"

		_, resp = client.UpdateCommand(cmd2)
		CheckBadRequestStatus(t, resp)

		cmd2.Id = cmd1.Id
		cmd2.TeamId = GenerateTestId()

		_, resp = client.UpdateCommand(cmd2)
		CheckBadRequestStatus(t, resp)

		cmd2.TeamId = team.Id

		_, resp = th.Client.UpdateCommand(cmd2)
		CheckNotFoundStatus(t, resp)
	})
	th.SystemAdminClient.Logout()
	_, resp := th.SystemAdminClient.UpdateCommand(cmd2)
	CheckUnauthorizedStatus(t, resp)
}

func TestMoveCommand(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	user := th.SystemAdminUser
	team := th.BasicTeam
	newTeam := th.CreateTeam()

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })

	cmd1 := &model.Command{
		CreatorId: user.Id,
		TeamId:    team.Id,
		URL:       "http://nowhere.com",
		Method:    model.CommandMethodPost,
		Trigger:   "trigger1",
	}

	rcmd1, _ := th.App.CreateCommand(cmd1)
	th.TestForSystemAdminAndLocal(t, func(t *testing.T, client *model.Client4) {

		ok, resp := client.MoveCommand(newTeam.Id, rcmd1.Id)
		CheckNoError(t, resp)
		require.True(t, ok)

		rcmd1, _ = th.App.GetCommand(rcmd1.Id)
		require.NotNil(t, rcmd1)
		require.Equal(t, newTeam.Id, rcmd1.TeamId)

		ok, resp = client.MoveCommand(newTeam.Id, "bogus")
		CheckBadRequestStatus(t, resp)
		require.False(t, ok)

		ok, resp = client.MoveCommand(GenerateTestId(), rcmd1.Id)
		CheckNotFoundStatus(t, resp)
		require.False(t, ok)
	})
	cmd2 := &model.Command{
		CreatorId: user.Id,
		TeamId:    team.Id,
		URL:       "http://nowhere.com",
		Method:    model.CommandMethodPost,
		Trigger:   "trigger2",
	}

	rcmd2, _ := th.App.CreateCommand(cmd2)

	_, resp := th.Client.MoveCommand(newTeam.Id, rcmd2.Id)
	CheckNotFoundStatus(t, resp)

	th.SystemAdminClient.Logout()
	_, resp = th.SystemAdminClient.MoveCommand(newTeam.Id, rcmd2.Id)
	CheckUnauthorizedStatus(t, resp)
}

func TestDeleteCommand(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	user := th.SystemAdminUser
	team := th.BasicTeam

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })

	cmd1 := &model.Command{
		CreatorId: user.Id,
		TeamId:    team.Id,
		URL:       "http://nowhere.com",
		Method:    model.CommandMethodPost,
		Trigger:   "trigger1",
	}

	th.TestForSystemAdminAndLocal(t, func(t *testing.T, client *model.Client4) {
		cmd1.Id = ""
		rcmd1, err := th.App.CreateCommand(cmd1)
		require.Nil(t, err)
		ok, resp := client.DeleteCommand(rcmd1.Id)
		CheckNoError(t, resp)

		require.True(t, ok)

		rcmd1, _ = th.App.GetCommand(rcmd1.Id)
		require.Nil(t, rcmd1)

		ok, resp = client.DeleteCommand("junk")
		CheckBadRequestStatus(t, resp)

		require.False(t, ok)

		_, resp = client.DeleteCommand(GenerateTestId())
		CheckNotFoundStatus(t, resp)
	})
	cmd2 := &model.Command{
		CreatorId: user.Id,
		TeamId:    team.Id,
		URL:       "http://nowhere.com",
		Method:    model.CommandMethodPost,
		Trigger:   "trigger2",
	}

	rcmd2, _ := th.App.CreateCommand(cmd2)

	_, resp := th.Client.DeleteCommand(rcmd2.Id)
	CheckNotFoundStatus(t, resp)

	th.SystemAdminClient.Logout()
	_, resp = th.SystemAdminClient.DeleteCommand(rcmd2.Id)
	CheckUnauthorizedStatus(t, resp)
}

func TestListCommands(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	Client := th.Client

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })

	newCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    th.BasicTeam.Id,
		URL:       "http://nowhere.com",
		Method:    model.CommandMethodPost,
		Trigger:   "custom_command"}

	_, resp := th.SystemAdminClient.CreateCommand(newCmd)
	CheckNoError(t, resp)

	th.TestForSystemAdminAndLocal(t, func(t *testing.T, c *model.Client4) {
		listCommands, resp := c.ListCommands(th.BasicTeam.Id, false)
		CheckNoError(t, resp)

		foundEcho := false
		foundCustom := false
		for _, command := range listCommands {
			if command.Trigger == "echo" {
				foundEcho = true
			}
			if command.Trigger == "custom_command" {
				foundCustom = true
			}
		}
		require.True(t, foundEcho, "Couldn't find echo command")
		require.True(t, foundCustom, "Should list the custom command")
	}, "ListSystemAndCustomCommands")

	th.TestForSystemAdminAndLocal(t, func(t *testing.T, c *model.Client4) {
		listCommands, resp := c.ListCommands(th.BasicTeam.Id, true)
		CheckNoError(t, resp)

		require.Len(t, listCommands, 1, "Should list just one custom command")
		require.Equal(t, listCommands[0].Trigger, "custom_command", "Wrong custom command trigger")
	}, "ListCustomOnlyCommands")

	t.Run("UserWithNoPermissionForCustomCommands", func(t *testing.T) {
		_, resp := Client.ListCommands(th.BasicTeam.Id, true)
		CheckForbiddenStatus(t, resp)
	})

	t.Run("RegularUserCanListOnlySystemCommands", func(t *testing.T) {
		listCommands, resp := Client.ListCommands(th.BasicTeam.Id, false)
		CheckNoError(t, resp)

		foundEcho := false
		foundCustom := false
		for _, command := range listCommands {
			if command.Trigger == "echo" {
				foundEcho = true
			}
			if command.Trigger == "custom_command" {
				foundCustom = true
			}
		}
		require.True(t, foundEcho, "Couldn't find echo command")
		require.False(t, foundCustom, "Should not list the custom command")
	})

	t.Run("NoMember", func(t *testing.T) {
		Client.Logout()
		user := th.CreateUser()
		th.SystemAdminClient.RemoveTeamMember(th.BasicTeam.Id, user.Id)
		Client.Login(user.Email, user.Password)
		_, resp := Client.ListCommands(th.BasicTeam.Id, false)
		CheckForbiddenStatus(t, resp)
		_, resp = Client.ListCommands(th.BasicTeam.Id, true)
		CheckForbiddenStatus(t, resp)
	})

	t.Run("NotLoggedIn", func(t *testing.T) {
		Client.Logout()
		_, resp := Client.ListCommands(th.BasicTeam.Id, false)
		CheckUnauthorizedStatus(t, resp)
		_, resp = Client.ListCommands(th.BasicTeam.Id, true)
		CheckUnauthorizedStatus(t, resp)
	})
}

func TestListAutocompleteCommands(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	Client := th.Client

	newCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    th.BasicTeam.Id,
		URL:       "http://nowhere.com",
		Method:    model.CommandMethodPost,
		Trigger:   "custom_command"}

	_, resp := th.SystemAdminClient.CreateCommand(newCmd)
	CheckNoError(t, resp)

	t.Run("ListAutocompleteCommandsOnly", func(t *testing.T) {
		listCommands, resp := th.SystemAdminClient.ListAutocompleteCommands(th.BasicTeam.Id)
		CheckNoError(t, resp)

		foundEcho := false
		foundCustom := false
		for _, command := range listCommands {
			if command.Trigger == "echo" {
				foundEcho = true
			}
			if command.Trigger == "custom_command" {
				foundCustom = true
			}
		}
		require.True(t, foundEcho, "Couldn't find echo command")
		require.False(t, foundCustom, "Should not list the custom command")
	})

	t.Run("RegularUserCanListOnlySystemCommands", func(t *testing.T) {
		listCommands, resp := Client.ListAutocompleteCommands(th.BasicTeam.Id)
		CheckNoError(t, resp)

		foundEcho := false
		foundCustom := false
		for _, command := range listCommands {
			if command.Trigger == "echo" {
				foundEcho = true
			}
			if command.Trigger == "custom_command" {
				foundCustom = true
			}
		}
		require.True(t, foundEcho, "Couldn't find echo command")
		require.False(t, foundCustom, "Should not list the custom command")
	})

	t.Run("NoMember", func(t *testing.T) {
		Client.Logout()
		user := th.CreateUser()
		th.SystemAdminClient.RemoveTeamMember(th.BasicTeam.Id, user.Id)
		Client.Login(user.Email, user.Password)
		_, resp := Client.ListAutocompleteCommands(th.BasicTeam.Id)
		CheckForbiddenStatus(t, resp)
	})

	t.Run("NotLoggedIn", func(t *testing.T) {
		Client.Logout()
		_, resp := Client.ListAutocompleteCommands(th.BasicTeam.Id)
		CheckUnauthorizedStatus(t, resp)
	})
}

func TestListCommandAutocompleteSuggestions(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	Client := th.Client

	newCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    th.BasicTeam.Id,
		URL:       "http://nowhere.com",
		Method:    model.CommandMethodPost,
		Trigger:   "custom_command"}

	_, resp := th.SystemAdminClient.CreateCommand(newCmd)
	CheckNoError(t, resp)

	t.Run("ListAutocompleteSuggestionsOnly", func(t *testing.T) {
		suggestions, resp := th.SystemAdminClient.ListCommandAutocompleteSuggestions("/", th.BasicTeam.Id)
		CheckNoError(t, resp)

		foundEcho := false
		foundShrug := false
		foundCustom := false
		for _, command := range suggestions {
			if command.Suggestion == "echo" {
				foundEcho = true
			}
			if command.Suggestion == "shrug" {
				foundShrug = true
			}
			if command.Suggestion == "custom_command" {
				foundCustom = true
			}
		}
		require.True(t, foundEcho, "Couldn't find echo command")
		require.True(t, foundShrug, "Couldn't find shrug command")
		require.False(t, foundCustom, "Should not list the custom command")
	})

	t.Run("ListAutocompleteSuggestionsOnlyWithInput", func(t *testing.T) {
		suggestions, resp := th.SystemAdminClient.ListCommandAutocompleteSuggestions("/e", th.BasicTeam.Id)
		CheckNoError(t, resp)

		foundEcho := false
		foundShrug := false
		for _, command := range suggestions {
			if command.Suggestion == "echo" {
				foundEcho = true
			}
			if command.Suggestion == "shrug" {
				foundShrug = true
			}
		}
		require.True(t, foundEcho, "Couldn't find echo command")
		require.False(t, foundShrug, "Should not list the shrug command")
	})

	t.Run("RegularUserCanListOnlySystemCommands", func(t *testing.T) {
		suggestions, resp := Client.ListCommandAutocompleteSuggestions("/", th.BasicTeam.Id)
		CheckNoError(t, resp)

		foundEcho := false
		foundCustom := false
		for _, suggestion := range suggestions {
			if suggestion.Suggestion == "echo" {
				foundEcho = true
			}
			if suggestion.Suggestion == "custom_command" {
				foundCustom = true
			}
		}
		require.True(t, foundEcho, "Couldn't find echo command")
		require.False(t, foundCustom, "Should not list the custom command")
	})

	t.Run("NoMember", func(t *testing.T) {
		Client.Logout()
		user := th.CreateUser()
		th.SystemAdminClient.RemoveTeamMember(th.BasicTeam.Id, user.Id)
		Client.Login(user.Email, user.Password)
		_, resp := Client.ListCommandAutocompleteSuggestions("/", th.BasicTeam.Id)
		CheckForbiddenStatus(t, resp)
	})

	t.Run("NotLoggedIn", func(t *testing.T) {
		Client.Logout()
		_, resp := Client.ListCommandAutocompleteSuggestions("/", th.BasicTeam.Id)
		CheckUnauthorizedStatus(t, resp)
	})
}

func TestGetCommand(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })

	newCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    th.BasicTeam.Id,
		URL:       "http://nowhere.com",
		Method:    model.CommandMethodPost,
		Trigger:   "roger"}

	newCmd, resp := th.SystemAdminClient.CreateCommand(newCmd)
	CheckNoError(t, resp)
	th.TestForSystemAdminAndLocal(t, func(t *testing.T, client *model.Client4) {

		t.Run("ValidId", func(t *testing.T) {
			cmd, resp := client.GetCommandById(newCmd.Id)
			CheckNoError(t, resp)

			require.Equal(t, newCmd.Id, cmd.Id)
			require.Equal(t, newCmd.CreatorId, cmd.CreatorId)
			require.Equal(t, newCmd.TeamId, cmd.TeamId)
			require.Equal(t, newCmd.URL, cmd.URL)
			require.Equal(t, newCmd.Method, cmd.Method)
			require.Equal(t, newCmd.Trigger, cmd.Trigger)
		})

		t.Run("InvalidId", func(t *testing.T) {
			_, resp := client.GetCommandById(strings.Repeat("z", len(newCmd.Id)))
			require.NotNil(t, resp.Error)
		})
	})
	t.Run("UserWithNoPermissionForCustomCommands", func(t *testing.T) {
		_, resp := th.Client.GetCommandById(newCmd.Id)
		CheckNotFoundStatus(t, resp)
	})

	t.Run("NoMember", func(t *testing.T) {
		th.Client.Logout()
		user := th.CreateUser()
		th.SystemAdminClient.RemoveTeamMember(th.BasicTeam.Id, user.Id)
		th.Client.Login(user.Email, user.Password)
		_, resp := th.Client.GetCommandById(newCmd.Id)
		CheckNotFoundStatus(t, resp)
	})

	t.Run("NotLoggedIn", func(t *testing.T) {
		th.Client.Logout()
		_, resp := th.Client.GetCommandById(newCmd.Id)
		CheckUnauthorizedStatus(t, resp)
	})
}

func TestRegenToken(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	Client := th.Client

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })

	newCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    th.BasicTeam.Id,
		URL:       "http://nowhere.com",
		Method:    model.CommandMethodPost,
		Trigger:   "trigger"}

	createdCmd, resp := th.SystemAdminClient.CreateCommand(newCmd)
	CheckNoError(t, resp)
	CheckCreatedStatus(t, resp)

	token, resp := th.SystemAdminClient.RegenCommandToken(createdCmd.Id)
	CheckNoError(t, resp)
	require.NotEqual(t, createdCmd.Token, token, "should update the token")

	token, resp = Client.RegenCommandToken(createdCmd.Id)
	CheckNotFoundStatus(t, resp)
	require.Empty(t, token, "should not return the token")
}

func TestExecuteInvalidCommand(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	Client := th.Client
	channel := th.BasicChannel

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	allowedInternalConnections := *th.App.Config().ServiceSettings.AllowedUntrustedInternalConnections
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
		th.App.UpdateConfig(func(cfg *model.Config) {
			cfg.ServiceSettings.AllowedUntrustedInternalConnections = &allowedInternalConnections
		})
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.AllowedUntrustedInternalConnections = "127.0.0.0/8" })

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := &model.CommandResponse{}

		if err := json.NewEncoder(w).Encode(rc); err != nil {
			mlog.Warn("Error while writing response", mlog.Err(err))
		}
	}))
	defer ts.Close()

	getCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    th.BasicTeam.Id,
		URL:       ts.URL,
		Method:    model.CommandMethodGet,
		Trigger:   "getcommand",
	}

	_, err := th.App.CreateCommand(getCmd)
	require.Nil(t, err, "failed to create get command")

	_, resp := Client.ExecuteCommand(channel.Id, "")
	CheckBadRequestStatus(t, resp)

	_, resp = Client.ExecuteCommand(channel.Id, "/")
	CheckBadRequestStatus(t, resp)

	_, resp = Client.ExecuteCommand(channel.Id, "getcommand")
	CheckBadRequestStatus(t, resp)

	_, resp = Client.ExecuteCommand(channel.Id, "/junk")
	CheckNotFoundStatus(t, resp)

	otherUser := th.CreateUser()
	Client.Login(otherUser.Email, otherUser.Password)

	_, resp = Client.ExecuteCommand(channel.Id, "/getcommand")
	CheckForbiddenStatus(t, resp)

	Client.Logout()

	_, resp = Client.ExecuteCommand(channel.Id, "/getcommand")
	CheckUnauthorizedStatus(t, resp)

	_, resp = th.SystemAdminClient.ExecuteCommand(channel.Id, "/getcommand")
	CheckNoError(t, resp)
}

func TestExecuteGetCommand(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	Client := th.Client
	channel := th.BasicChannel

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	allowedInternalConnections := *th.App.Config().ServiceSettings.AllowedUntrustedInternalConnections
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
		th.App.UpdateConfig(func(cfg *model.Config) {
			cfg.ServiceSettings.AllowedUntrustedInternalConnections = &allowedInternalConnections
		})
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.AllowedUntrustedInternalConnections = "127.0.0.0/8" })

	token := model.NewId()
	expectedCommandResponse := &model.CommandResponse{
		Text:         "test get command response",
		ResponseType: model.CommandResponseTypeInChannel,
		Type:         "custom_test",
		Props:        map[string]interface{}{"someprop": "somevalue"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)

		values, err := url.ParseQuery(r.URL.RawQuery)
		require.NoError(t, err)

		require.Equal(t, token, values.Get("token"))
		require.Equal(t, th.BasicTeam.Name, values.Get("team_domain"))
		require.Equal(t, "ourCommand", values.Get("cmd"))

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expectedCommandResponse); err != nil {
			mlog.Warn("Error while writing response", mlog.Err(err))
		}
	}))
	defer ts.Close()

	getCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    th.BasicTeam.Id,
		URL:       ts.URL + "/?cmd=ourCommand",
		Method:    model.CommandMethodGet,
		Trigger:   "getcommand",
		Token:     token,
	}

	_, err := th.App.CreateCommand(getCmd)
	require.Nil(t, err, "failed to create get command")

	commandResponse, resp := Client.ExecuteCommand(channel.Id, "/getcommand")
	CheckNoError(t, resp)
	assert.True(t, len(commandResponse.TriggerId) == 26)

	expectedCommandResponse.TriggerId = commandResponse.TriggerId
	require.Equal(t, expectedCommandResponse, commandResponse)
}

func TestExecutePostCommand(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	Client := th.Client
	channel := th.BasicChannel

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	allowedInternalConnections := *th.App.Config().ServiceSettings.AllowedUntrustedInternalConnections
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
		th.App.UpdateConfig(func(cfg *model.Config) {
			cfg.ServiceSettings.AllowedUntrustedInternalConnections = &allowedInternalConnections
		})
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.AllowedUntrustedInternalConnections = "127.0.0.0/8" })

	token := model.NewId()
	expectedCommandResponse := &model.CommandResponse{
		Text:         "test post command response",
		ResponseType: model.CommandResponseTypeInChannel,
		Type:         "custom_test",
		Props:        map[string]interface{}{"someprop": "somevalue"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)

		r.ParseForm()

		require.Equal(t, token, r.FormValue("token"))
		require.Equal(t, th.BasicTeam.Name, r.FormValue("team_domain"))

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expectedCommandResponse); err != nil {
			mlog.Warn("Error while writing response", mlog.Err(err))
		}
	}))
	defer ts.Close()

	postCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    th.BasicTeam.Id,
		URL:       ts.URL,
		Method:    model.CommandMethodPost,
		Trigger:   "postcommand",
		Token:     token,
	}

	_, err := th.App.CreateCommand(postCmd)
	require.Nil(t, err, "failed to create get command")

	commandResponse, resp := Client.ExecuteCommand(channel.Id, "/postcommand")
	CheckNoError(t, resp)
	assert.True(t, len(commandResponse.TriggerId) == 26)

	expectedCommandResponse.TriggerId = commandResponse.TriggerId
	require.Equal(t, expectedCommandResponse, commandResponse)
}

func TestExecuteCommandAgainstChannelOnAnotherTeam(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	Client := th.Client
	channel := th.BasicChannel

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	allowedInternalConnections := *th.App.Config().ServiceSettings.AllowedUntrustedInternalConnections
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
		th.App.UpdateConfig(func(cfg *model.Config) {
			cfg.ServiceSettings.AllowedUntrustedInternalConnections = &allowedInternalConnections
		})
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })
	th.App.UpdateConfig(func(cfg *model.Config) {
		*cfg.ServiceSettings.AllowedUntrustedInternalConnections = "localhost,127.0.0.1"
	})

	expectedCommandResponse := &model.CommandResponse{
		Text:         "test post command response",
		ResponseType: model.CommandResponseTypeInChannel,
		Type:         "custom_test",
		Props:        map[string]interface{}{"someprop": "somevalue"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expectedCommandResponse); err != nil {
			mlog.Warn("Error while writing response", mlog.Err(err))
		}
	}))
	defer ts.Close()

	// create a slash command on some other team where we have permission to do so
	team2 := th.CreateTeam()
	postCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    team2.Id,
		URL:       ts.URL,
		Method:    model.CommandMethodPost,
		Trigger:   "postcommand",
	}
	_, err := th.App.CreateCommand(postCmd)
	require.Nil(t, err, "failed to create post command")

	// the execute command endpoint will always search for the command by trigger and team id, inferring team id from the
	// channel id, so there is no way to use that slash command on a channel that belongs to some other team
	_, resp := Client.ExecuteCommand(channel.Id, "/postcommand")
	CheckNotFoundStatus(t, resp)
}

func TestExecuteCommandAgainstChannelUserIsNotIn(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	client := th.Client

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	allowedInternalConnections := *th.App.Config().ServiceSettings.AllowedUntrustedInternalConnections
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
		th.App.UpdateConfig(func(cfg *model.Config) {
			cfg.ServiceSettings.AllowedUntrustedInternalConnections = &allowedInternalConnections
		})
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })
	th.App.UpdateConfig(func(cfg *model.Config) {
		*cfg.ServiceSettings.AllowedUntrustedInternalConnections = "localhost,127.0.0.1"
	})

	expectedCommandResponse := &model.CommandResponse{
		Text:         "test post command response",
		ResponseType: model.CommandResponseTypeInChannel,
		Type:         "custom_test",
		Props:        map[string]interface{}{"someprop": "somevalue"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expectedCommandResponse); err != nil {
			mlog.Warn("Error while writing response", mlog.Err(err))
		}
	}))
	defer ts.Close()

	// create a slash command on some other team where we have permission to do so
	team2 := th.CreateTeam()
	postCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    team2.Id,
		URL:       ts.URL,
		Method:    model.CommandMethodPost,
		Trigger:   "postcommand",
	}
	_, err := th.App.CreateCommand(postCmd)
	require.Nil(t, err, "failed to create post command")

	// make a channel on that team, ensuring that our test user isn't in it
	channel2 := th.CreateChannelWithClientAndTeam(client, model.ChannelTypeOpen, team2.Id)
	success, _ := client.RemoveUserFromChannel(channel2.Id, th.BasicUser.Id)
	require.True(t, success, "Failed to remove user from channel")

	// we should not be able to run the slash command in channel2, because we aren't in it
	_, resp := client.ExecuteCommandWithTeam(channel2.Id, team2.Id, "/postcommand")
	CheckForbiddenStatus(t, resp)
}

func TestExecuteCommandInDirectMessageChannel(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	client := th.Client

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	allowedInternalConnections := *th.App.Config().ServiceSettings.AllowedUntrustedInternalConnections
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
		th.App.UpdateConfig(func(cfg *model.Config) {
			cfg.ServiceSettings.AllowedUntrustedInternalConnections = &allowedInternalConnections
		})
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })
	th.App.UpdateConfig(func(cfg *model.Config) {
		*cfg.ServiceSettings.AllowedUntrustedInternalConnections = "localhost,127.0.0.1"
	})

	// create a team that the user isn't a part of
	team2 := th.CreateTeam()

	expectedCommandResponse := &model.CommandResponse{
		Text:         "test post command response",
		ResponseType: model.CommandResponseTypeInChannel,
		Type:         "custom_test",
		Props:        map[string]interface{}{"someprop": "somevalue"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expectedCommandResponse); err != nil {
			mlog.Warn("Error while writing response", mlog.Err(err))
		}
	}))
	defer ts.Close()

	// create a slash command on some other team where we have permission to do so
	postCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    team2.Id,
		URL:       ts.URL,
		Method:    model.CommandMethodPost,
		Trigger:   "postcommand",
	}
	_, err := th.App.CreateCommand(postCmd)
	require.Nil(t, err, "failed to create post command")

	// make a direct message channel
	dmChannel, response := client.CreateDirectChannel(th.BasicUser.Id, th.BasicUser2.Id)
	CheckCreatedStatus(t, response)

	// we should be able to run the slash command in the DM channel
	_, resp := client.ExecuteCommandWithTeam(dmChannel.Id, team2.Id, "/postcommand")
	CheckOKStatus(t, resp)

	// but we can't run the slash command in the DM channel if we sub in some other team's id
	_, resp = client.ExecuteCommandWithTeam(dmChannel.Id, th.BasicTeam.Id, "/postcommand")
	CheckNotFoundStatus(t, resp)
}

func TestExecuteCommandInTeamUserIsNotOn(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	client := th.Client

	enableCommands := *th.App.Config().ServiceSettings.EnableCommands
	allowedInternalConnections := *th.App.Config().ServiceSettings.AllowedUntrustedInternalConnections
	defer func() {
		th.App.UpdateConfig(func(cfg *model.Config) { cfg.ServiceSettings.EnableCommands = &enableCommands })
		th.App.UpdateConfig(func(cfg *model.Config) {
			cfg.ServiceSettings.AllowedUntrustedInternalConnections = &allowedInternalConnections
		})
	}()
	th.App.UpdateConfig(func(cfg *model.Config) { *cfg.ServiceSettings.EnableCommands = true })
	th.App.UpdateConfig(func(cfg *model.Config) {
		*cfg.ServiceSettings.AllowedUntrustedInternalConnections = "localhost,127.0.0.1"
	})

	// create a team that the user isn't a part of
	team2 := th.CreateTeam()

	expectedCommandResponse := &model.CommandResponse{
		Text:         "test post command response",
		ResponseType: model.CommandResponseTypeInChannel,
		Type:         "custom_test",
		Props:        map[string]interface{}{"someprop": "somevalue"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		r.ParseForm()
		require.Equal(t, team2.Name, r.FormValue("team_domain"))

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expectedCommandResponse); err != nil {
			mlog.Warn("Error while writing response", mlog.Err(err))
		}
	}))
	defer ts.Close()

	// create a slash command on that team
	postCmd := &model.Command{
		CreatorId: th.BasicUser.Id,
		TeamId:    team2.Id,
		URL:       ts.URL,
		Method:    model.CommandMethodPost,
		Trigger:   "postcommand",
	}
	_, err := th.App.CreateCommand(postCmd)
	require.Nil(t, err, "failed to create post command")

	// make a direct message channel
	dmChannel, response := client.CreateDirectChannel(th.BasicUser.Id, th.BasicUser2.Id)
	CheckCreatedStatus(t, response)

	// we should be able to run the slash command in the DM channel
	_, resp := client.ExecuteCommandWithTeam(dmChannel.Id, team2.Id, "/postcommand")
	CheckOKStatus(t, resp)

	// if the user is removed from the team, they should NOT be able to run the slash command in the DM channel
	success, _ := client.RemoveTeamMember(team2.Id, th.BasicUser.Id)
	require.True(t, success, "Failed to remove user from team")

	_, resp = client.ExecuteCommandWithTeam(dmChannel.Id, team2.Id, "/postcommand")
	CheckForbiddenStatus(t, resp)

	// if we omit the team id from the request, the slash command will fail because this is a DM channel, and the
	// team id can't be inherited from the channel
	_, resp = client.ExecuteCommand(dmChannel.Id, "/postcommand")
	CheckForbiddenStatus(t, resp)
}
