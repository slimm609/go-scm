// Copyright 2017 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gitlab

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/slimm609/go-scm/scm"
	"github.com/slimm609/go-scm/scm/driver/internal/null"
)

type userService struct {
	client *wrapper
}

func (s *userService) CreateToken(context.Context, string, string) (*scm.UserToken, *scm.Response, error) {
	return nil, nil, scm.ErrNotSupported
}

func (s *userService) DeleteToken(context.Context, int64) (*scm.Response, error) {
	return nil, scm.ErrNotSupported
}

func (s *userService) Find(ctx context.Context) (*scm.User, *scm.Response, error) {
	out := new(user)
	res, err := s.client.do(ctx, "GET", "api/v4/user", nil, out)
	return convertUser(out), res, err
}

func (s *userService) FindLogin(ctx context.Context, login string) (*scm.User, *scm.Response, error) {
	var resp *scm.Response
	var err error
	var path string
	firstRun := false
	opts := scm.ListOptions{
		Page: 1,
	}
	for !firstRun || (resp != nil && opts.Page <= resp.Page.Last) {
		out := []*user{}
		_, err = strconv.Atoi(login)
		if err != nil {
			path = fmt.Sprintf("api/v4/users?search=%s&%s", login, encodeListOptions(opts))
			resp, err = s.client.do(ctx, "GET", path, nil, &out)
		} else {
			var single *user
			path = fmt.Sprintf("api/v4/users/%s", login)
			fmt.Printf("path: %s\n", path)
			resp, err = s.client.do(ctx, "GET", path, nil, &single)
			out = []*user{single}
		}
		if err != nil {
			return nil, nil, err
		}
		firstRun = true
		for _, u := range out {
			if strings.EqualFold(u.Username, login) {
				return convertUser(u), resp, err
			}
		}
		opts.Page++
	}
	return nil, resp, scm.ErrNotFound
}

func (s *userService) FindEmail(ctx context.Context) (string, *scm.Response, error) {
	user, res, err := s.Find(ctx)
	return user.Email, res, err
}

func (s *userService) ListInvitations(context.Context) ([]*scm.Invitation, *scm.Response, error) {
	return nil, nil, scm.ErrNotSupported
}

func (s *userService) AcceptInvitation(context.Context, int64) (*scm.Response, error) {
	return nil, scm.ErrNotSupported
}

type user struct {
	ID       int         `json:"id"`
	Username string      `json:"username"`
	Name     string      `json:"name"`
	Email    null.String `json:"email"`
	Avatar   string      `json:"avatar_url"`
}

func convertUser(from *user) *scm.User {
	return &scm.User{
		ID:     from.ID,
		Avatar: from.Avatar,
		Email:  from.Email.String,
		Login:  from.Username,
		Name:   from.Name,
	}
}

func convertUserList(users []*user) []scm.User {
	dst := []scm.User{}
	for _, src := range users {
		dst = append(dst, *convertUser(src))
	}
	return dst
}
