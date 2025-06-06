// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// PathsReducer is used to reduce the paths in an activity tree according to predefined heuristics
type PathsReducer struct {
	patterns []PatternReducer
}

// PatternReducer is used to reduce the paths in an activity tree according to a given pattern
type PatternReducer struct {
	Pattern  *regexp.Regexp
	Hint     string
	PreCheck func(path string, fileEvent *model.FileEvent) bool
	Callback func(ctx *callbackContext)
}

// callbackContext is the input struct for the callback function
type callbackContext struct {
	groups      []int
	path        string
	fileEvent   *model.FileEvent
	processNode *ProcessNode
}

func (cc *callbackContext) getGroup(index int) (int, int) {
	return cc.groups[index*2], cc.groups[index*2+1]
}

func (cc *callbackContext) replaceBy(start, end int, replaceBy string) {
	left := cc.path[:start]
	right := cc.path[end:]

	var b strings.Builder
	b.Grow(len(left) + len(replaceBy) + len(right))
	b.WriteString(left)
	b.WriteString(replaceBy)
	b.WriteString(right)
	cc.path = b.String()
}

// NewPathsReducer returns a new PathsReducer
func NewPathsReducer() *PathsReducer {
	return &PathsReducer{
		patterns: getPathsReducerPatterns(),
	}
}

// ReducePath reduces a path according to the predefined heuristics
func (r *PathsReducer) ReducePath(path string, fileEvent *model.FileEvent, node *ProcessNode) string {
	var ctx *callbackContext

	for _, pattern := range r.patterns {
		currentPath := path
		if ctx != nil {
			currentPath = ctx.path
		}

		if pattern.PreCheck != nil && fileEvent != nil && !pattern.PreCheck(currentPath, fileEvent) {
			continue
		}

		if pattern.Hint != "" && !strings.Contains(currentPath, pattern.Hint) {
			continue
		}

		allMatches := pattern.Pattern.FindAllStringSubmatchIndex(currentPath, -1)

		if len(allMatches) == 0 {
			continue
		}

		// if no regex matches, we fully skip the callbackContext allocation
		if ctx == nil {
			ctx = &callbackContext{
				path:        path,
				fileEvent:   fileEvent,
				processNode: node,
			}
		}

		for matchSet := len(allMatches) - 1; matchSet >= 0; matchSet-- {
			if pattern.Callback != nil {
				ctx.groups = allMatches[matchSet]
				pattern.Callback(ctx)
			}
		}
	}

	if ctx != nil {
		return ctx.path
	}

	return path
}

// getPathsReducerPatterns returns the patterns used to reduce the paths in an activity tree
func getPathsReducerPatterns() []PatternReducer {
	return []PatternReducer{
		{
			Pattern: regexp.MustCompile(`/proc/(\d+)/`), // process PID
			Hint:    "proc",
			Callback: func(ctx *callbackContext) {
				start, end := ctx.getGroup(1)
				// compute pid from path
				pid, err := strconv.ParseUint(ctx.path[start:end], 10, 32)
				if err != nil {
					return
				}
				// replace the pid in the path between start and end with a * only if the replaced pid is not the pid of the process node
				if ctx.processNode.Process.Pid == uint32(pid) {
					ctx.replaceBy(start, end, "self")
				} else {
					ctx.replaceBy(start, end, "*")
				}
			},
		},
		{
			Pattern: regexp.MustCompile(`/task/(\d+)/`), // process TID
			Hint:    "task",
			Callback: func(ctx *callbackContext) {
				start, end := ctx.getGroup(1)
				ctx.replaceBy(start, end, "*")
			},
		},
		{
			Pattern: regexp.MustCompile(`kubepods-([^/]*)\.(?:slice|scope)`), // kubernetes cgroup
			Hint:    "kubepods",
			PreCheck: func(_ string, fileEvent *model.FileEvent) bool {
				return fileEvent.Filesystem == "sysfs"
			},
			Callback: func(ctx *callbackContext) {
				start, end := ctx.getGroup(1)
				ctx.replaceBy(start, end, "*")
			},
		},
		{
			Pattern: regexp.MustCompile(`cri-containerd-([^/]*)\.(?:slice|scope)`), // kubernetes cgroup
			Hint:    "cri-containerd",
			PreCheck: func(_ string, fileEvent *model.FileEvent) bool {
				return fileEvent.Filesystem == "sysfs"
			},
			Callback: func(ctx *callbackContext) {
				start, end := ctx.getGroup(1)
				ctx.replaceBy(start, end, "*")
			},
		},
		{
			Pattern: regexp.MustCompile(containerutils.ContainerIDPatternStr), // container ID
			PreCheck: func(path string, _ *model.FileEvent) bool {
				var count int
				for _, c := range []byte(path) {
					if isHexChar(c) || c == '-' {
						count++
						if count >= 28 { // 28 is the minimal length of a container ID
							return true
						}
					} else {
						count = 0
					}
				}
				return false
			},
			Callback: func(ctx *callbackContext) {
				start, end := ctx.getGroup(0)
				ctx.replaceBy(start, end, "*")
			},
		},
		{
			Pattern: regexp.MustCompile(`/sys/devices/virtual/block/(?:dm-|loop)([0-9]+)`), // block devices
			Hint:    "devices",
			PreCheck: func(_ string, fileEvent *model.FileEvent) bool {
				return fileEvent.Filesystem == "sysfs"
			},
			Callback: func(ctx *callbackContext) {
				start, end := ctx.getGroup(1)
				ctx.replaceBy(start, end, "*")
			},
		},
		{
			Pattern: regexp.MustCompile(`secrets/kubernetes\.io/serviceaccount/([0-9._]+)`), // service account token date
			Hint:    "serviceaccount",
			Callback: func(ctx *callbackContext) {
				start, end := ctx.getGroup(1)
				ctx.replaceBy(start, end, "*")
			},
		},
	}
}

func isHexChar(c byte) bool {
	return ('0' <= c && c <= '9') ||
		('a' <= c && c <= 'f') ||
		('A' <= c && c <= 'F')
}
