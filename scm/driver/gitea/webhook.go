// Copyright 2017 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gitea

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"code.gitea.io/sdk/gitea"

	"github.com/slimm609/go-scm/pkg/hmac"
	"github.com/slimm609/go-scm/scm"
)

type webhookService struct {
	client *wrapper
}

func (s *webhookService) Parse(req *http.Request, fn scm.SecretFunc) (scm.Webhook, error) {
	data, err := ioutil.ReadAll(
		io.LimitReader(req.Body, 10000000),
	)
	if err != nil {
		return nil, err
	}

	secret := ""
	var hook scm.Webhook
	event := req.Header.Get("X-Gitea-Event")
	switch event {
	case "push":
		var push *pushHook
		hook, push, err = s.parsePushHook(data)
		secret = push.Secret
	case "create":
		hook, err = s.parseCreateHook(data)
	case "delete":
		hook, err = s.parseDeleteHook(data)
	case "issues":
		hook, err = s.parseIssueHook(data)
	case "issue_comment":
		hook, err = s.parseIssueCommentHook(data)
	case "pull_request":
		hook, err = s.parsePullRequestHook(data)
	case "reviewed":
		hook, err = s.parsePullRequestReviewHook(data)
	default:
		return nil, scm.UnknownWebhook{Event: event}
	}
	if err != nil {
		return nil, err
	}

	if secret == "" {
		secret = req.FormValue("secret")
	}

	// get the gitea signature key to verify the payload
	// signature. If no key is provided, no validation
	// is performed.
	key, err := fn(hook)
	if err != nil {
		return hook, err
	} else if key == "" {
		return hook, nil
	}

	signature := req.Header.Get("X-Gitea-Signature")

	// fail if no signature passed
	if signature == "" && secret == "" {
		return hook, scm.ErrSignatureInvalid
	}

	// test signature if header not set and secret is in payload
	if signature == "" && secret != "" && secret != key {
		return hook, scm.ErrSignatureInvalid
	}

	// test signature using header
	if signature != "" && !hmac.Validate(sha256.New, data, []byte(key), signature) {
		return hook, scm.ErrSignatureInvalid
	}

	return hook, nil
}

func (s *webhookService) parsePushHook(data []byte) (scm.Webhook, *pushHook, error) {
	dst := new(pushHook)
	err := json.Unmarshal(data, dst)
	return convertPushHook(dst), dst, err
}

func (s *webhookService) parseCreateHook(data []byte) (scm.Webhook, error) {
	dst := new(createHook)
	err := json.Unmarshal(data, dst)
	switch dst.RefType {
	case "tag":
		return convertTagHook(dst, scm.ActionCreate), err
	case "branch":
		return convertBranchHook(dst, scm.ActionCreate), err
	default:
		return nil, scm.UnknownWebhook{Event: dst.RefType}
	}
}

func (s *webhookService) parseDeleteHook(data []byte) (scm.Webhook, error) {
	dst := new(createHook)
	err := json.Unmarshal(data, dst)
	switch dst.RefType {
	case "tag":
		return convertTagHook(dst, scm.ActionDelete), err
	case "branch":
		return convertBranchHook(dst, scm.ActionDelete), err
	default:
		return nil, scm.UnknownWebhook{Event: dst.RefType}
	}
}

func (s *webhookService) parseIssueHook(data []byte) (scm.Webhook, error) {
	dst := new(issueHook)
	err := json.Unmarshal(data, dst)
	return convertIssueHook(dst), err
}

func (s *webhookService) parseIssueCommentHook(data []byte) (scm.Webhook, error) {
	dst := new(issueHook)
	err := json.Unmarshal(data, dst)
	if dst.Issue.PullRequest != nil {
		return convertPullRequestCommentHook(dst), err
	}
	return convertIssueCommentHook(dst), err
}

func (s *webhookService) parsePullRequestHook(data []byte) (scm.Webhook, error) {
	dst := new(pullRequestHook)
	err := json.Unmarshal(data, dst)
	return convertPullRequestHook(dst), err
}

func (s *webhookService) parsePullRequestReviewHook(data []byte) (scm.Webhook, error) {
	dst := new(pullRequestReviewHook)
	err := json.Unmarshal(data, dst)
	return convertPullRequestReviewHook(dst), err
}

//
// native data structures
//

type (
	// gitea push webhook payload
	pushHook struct {
		Secret     string           `json:"secret"`
		Ref        string           `json:"ref"`
		Before     string           `json:"before"`
		After      string           `json:"after"`
		Compare    string           `json:"compare_url"`
		Commits    []commit         `json:"commits"`
		Repository gitea.Repository `json:"repository"`
		Pusher     gitea.User       `json:"pusher"`
		Sender     gitea.User       `json:"sender"`
	}

	// gitea create webhook payload
	createHook struct {
		Ref           string           `json:"ref"`
		RefType       string           `json:"ref_type"`
		Sha           string           `json:"sha"`
		DefaultBranch string           `json:"default_branch"`
		Repository    gitea.Repository `json:"repository"`
		Sender        gitea.User       `json:"sender"`
	}

	// gitea issue webhook payload
	issueHook struct {
		Action     string           `json:"action"`
		Issue      gitea.Issue      `json:"issue"`
		Comment    gitea.Comment    `json:"comment"`
		Repository gitea.Repository `json:"repository"`
		Sender     gitea.User       `json:"sender"`
	}

	// gitea pull request webhook payload
	pullRequestHook struct {
		Action      string            `json:"action"`
		Number      int               `json:"number"`
		PullRequest gitea.PullRequest `json:"pull_request"`
		Repository  gitea.Repository  `json:"repository"`
		Sender      gitea.User        `json:"sender"`
	}

	// gitea pull request review webhook payload
	pullRequestReviewHook struct {
		Action      string                   `json:"action"`
		Number      int                      `json:"number"`
		PullRequest gitea.PullRequest        `json:"pull_request"`
		Repository  gitea.Repository         `json:"repository"`
		Sender      gitea.User               `json:"sender"`
		Review      pullRequestReviewPayload `json:"review"`
	}

	// gitea pull request review webhook sub-payload
	pullRequestReviewPayload struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
)

//
// native data structure conversion
//

func convertTagHook(dst *createHook, action scm.Action) *scm.TagHook {
	return &scm.TagHook{
		Action: action,
		Ref: scm.Reference{
			Name: dst.Ref,
			Sha:  dst.Sha,
		},
		Repo:   *convertRepository(&dst.Repository),
		Sender: *convertUser(&dst.Sender),
	}
}

func convertBranchHook(dst *createHook, action scm.Action) *scm.BranchHook {
	return &scm.BranchHook{
		Action: action,
		Ref: scm.Reference{
			Name: dst.Ref,
		},
		Repo:   *convertRepository(&dst.Repository),
		Sender: *convertUser(&dst.Sender),
	}
}

func convertPushHook(dst *pushHook) *scm.PushHook {
	if len(dst.Commits) > 0 {
		return &scm.PushHook{
			Ref: dst.Ref,
			Commit: scm.Commit{
				Sha:     dst.After,
				Message: dst.Commits[0].Message,
				Link:    dst.Compare,
				Author: scm.Signature{
					Login: dst.Commits[0].Author.Username,
					Email: dst.Commits[0].Author.Email,
					Name:  dst.Commits[0].Author.Name,
					Date:  dst.Commits[0].Timestamp,
				},
				Committer: scm.Signature{
					Login: dst.Commits[0].Committer.Username,
					Email: dst.Commits[0].Committer.Email,
					Name:  dst.Commits[0].Committer.Name,
					Date:  dst.Commits[0].Timestamp,
				},
			},
			Repo:   *convertRepository(&dst.Repository),
			Sender: *convertUser(&dst.Sender),
		}
	}
	return &scm.PushHook{
		Ref: dst.Ref,
		Commit: scm.Commit{
			Sha:  dst.After,
			Link: dst.Compare,
			Author: scm.Signature{
				Login: dst.Pusher.UserName,
				Email: dst.Pusher.Email,
				Name:  dst.Pusher.FullName,
			},
			Committer: scm.Signature{
				Login: dst.Pusher.UserName,
				Email: dst.Pusher.Email,
				Name:  dst.Pusher.FullName,
			},
		},
		Repo:   *convertRepository(&dst.Repository),
		Sender: *convertUser(&dst.Sender),
	}
}

func convertPullRequestHook(dst *pullRequestHook) *scm.PullRequestHook {
	return &scm.PullRequestHook{
		Action:      convertAction(dst.Action),
		PullRequest: *convertPullRequest(&dst.PullRequest),
		Repo:        *convertRepository(&dst.Repository),
		Sender:      *convertUser(&dst.Sender),
	}
}

func convertPullRequestReviewPayload(dst *pullRequestReviewHook) *scm.Review {
	return &scm.Review{
		Body:   dst.Review.Content,
		Author: *convertUser(&dst.Sender),
	}
}

func convertPullRequestReviewHook(dst *pullRequestReviewHook) *scm.ReviewHook {
	return &scm.ReviewHook{
		Action:      convertReviewAction(dst.Review.Type),
		PullRequest: *convertPullRequest(&dst.PullRequest),
		Repo:        *convertRepository(&dst.Repository),
		Review:      *convertPullRequestReviewPayload(dst),
	}
}

func convertPullRequestCommentHook(dst *issueHook) *scm.PullRequestCommentHook {
	return &scm.PullRequestCommentHook{
		Action:      convertAction(dst.Action),
		PullRequest: *convertPullRequestFromIssue(&dst.Issue),
		Comment:     *convertIssueComment(&dst.Comment),
		Repo:        *convertRepository(&dst.Repository),
		Sender:      *convertUser(&dst.Sender),
	}
}

func convertIssueHook(dst *issueHook) *scm.IssueHook {
	return &scm.IssueHook{
		Action: convertAction(dst.Action),
		Issue:  *convertIssue(&dst.Issue),
		Repo:   *convertRepository(&dst.Repository),
		Sender: *convertUser(&dst.Sender),
	}
}

func convertIssueCommentHook(dst *issueHook) *scm.IssueCommentHook {
	return &scm.IssueCommentHook{
		Action:  convertAction(dst.Action),
		Issue:   *convertIssue(&dst.Issue),
		Comment: *convertIssueComment(&dst.Comment),
		Repo:    *convertRepository(&dst.Repository),
		Sender:  *convertUser(&dst.Sender),
	}
}

func convertReviewAction(src string) (action scm.Action) {
	switch src {
	case "pull_request_review_approved":
		return scm.ActionSubmitted
	case "pull_request_review_comment":
		return scm.ActionEdited
	case "pull_request_review_rejected":
		return scm.ActionDismissed
	default:
		return
	}
}

func convertAction(src string) (action scm.Action) {
	switch src {
	case "create", "created":
		return scm.ActionCreate
	case "delete", "deleted":
		return scm.ActionDelete
	case "update", "updated", "edit", "edited":
		return scm.ActionUpdate
	case "open", "opened":
		return scm.ActionOpen
	case "reopen", "reopened":
		return scm.ActionReopen
	case "close", "closed":
		return scm.ActionClose
	case "label", "labeled", "label_updated":
		return scm.ActionLabel
	case "unlabel", "unlabeled", "label_cleared":
		return scm.ActionUnlabel
	case "merge", "merged":
		return scm.ActionMerge
	case "synchronize", "synchronized":
		return scm.ActionSync
	case "assigned":
		return scm.ActionAssigned
	case "unassigned":
		return scm.ActionUnassigned
	case "reviewed":
		return scm.ActionSubmitted
	default:
		return
	}
}
