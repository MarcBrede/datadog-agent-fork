// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"sort"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestExtractJob(t *testing.T) {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	startTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 31, 0, 0, time.UTC))
	completionTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 35, 0, 0, time.UTC))
	lastTransitionTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 35, 0, 0, time.UTC))

	tests := map[string]struct {
		input             batchv1.Job
		labelsAsTags      map[string]string
		annotationsAsTags map[string]string
		expected          model.Job
	}{
		"job started by cronjob (in progress)": {
			input: batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"annotation": "my-annotation",
					},
					CreationTimestamp: creationTime,
					Labels: map[string]string{
						"controller-uid": "43739057-c6d7-4a5e-ab63-d0c8844e5272",
						"app":            "my-app",
					},
					Name:      "job",
					Namespace: "project",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "batch/v1beta1",
							Controller: pointer.Ptr(true),
							Kind:       "CronJob",
							Name:       "test-job",
							UID:        "d0326ca4-d405-4fe9-99b5-7bfc4a6722b6",
						},
					},
					ResourceVersion: "220021511",
					UID:             types.UID("8893e7a0-fc49-4627-b695-3ed47074ecba"),
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: pointer.Ptr(int32(6)),
					Completions:  pointer.Ptr(int32(1)),
					Parallelism:  pointer.Ptr(int32(1)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"controller-uid": "43739057-c6d7-4a5e-ab63-d0c8844e5272",
						},
					},
				},
				Status: batchv1.JobStatus{
					Active:    1,
					StartTime: &startTime,
				},
			},
			labelsAsTags: map[string]string{
				"app": "application",
			},
			annotationsAsTags: map[string]string{
				"annotation": "annotation_key",
			},
			expected: model.Job{
				Metadata: &model.Metadata{
					Annotations:       []string{"annotation:my-annotation"},
					CreationTimestamp: creationTime.Unix(),
					Labels:            []string{"controller-uid:43739057-c6d7-4a5e-ab63-d0c8844e5272", "app:my-app"},
					Name:              "job",
					Namespace:         "project",
					OwnerReferences: []*model.OwnerReference{
						{
							Kind: "CronJob",
							Name: "test-job",
							Uid:  "d0326ca4-d405-4fe9-99b5-7bfc4a6722b6",
						},
					},
					ResourceVersion: "220021511",
					Uid:             "8893e7a0-fc49-4627-b695-3ed47074ecba",
				},
				Spec: &model.JobSpec{
					BackoffLimit: 6,
					Completions:  1,
					Parallelism:  1,
					Selectors: []*model.LabelSelectorRequirement{
						{
							Key:      "controller-uid",
							Operator: "In",
							Values:   []string{"43739057-c6d7-4a5e-ab63-d0c8844e5272"},
						},
					},
				},
				Status: &model.JobStatus{
					Active:    1,
					StartTime: startTime.Unix(),
				},
				Tags: []string{
					"application:my-app",
					"annotation_key:my-annotation",
				},
			},
		},
		"job started by cronjob (completed)": {
			input: batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"annotation": "my-annotation",
					},
					CreationTimestamp: creationTime,
					Labels:            map[string]string{"controller-uid": "43739057-c6d7-4a5e-ab63-d0c8844e5272"},
					Name:              "job",
					Namespace:         "project",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "batch/v1beta1",
							Controller: pointer.Ptr(true),
							Kind:       "CronJob",
							Name:       "test-job",
							UID:        "d0326ca4-d405-4fe9-99b5-7bfc4a6722b6",
						},
					},
					ResourceVersion: "220021511",
					UID:             types.UID("8893e7a0-fc49-4627-b695-3ed47074ecba"),
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: pointer.Ptr(int32(6)),
					Completions:  pointer.Ptr(int32(1)),
					Parallelism:  pointer.Ptr(int32(1)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"controller-uid": "43739057-c6d7-4a5e-ab63-d0c8844e5272",
						},
					},
				},
				Status: batchv1.JobStatus{
					CompletionTime: &completionTime,
					Conditions: []batchv1.JobCondition{
						{
							LastProbeTime:      lastTransitionTime,
							LastTransitionTime: lastTransitionTime,
							Status:             corev1.ConditionTrue,
							Type:               batchv1.JobComplete,
						},
					},
					Succeeded: 1,
					StartTime: &startTime,
				},
			},
			expected: model.Job{
				Metadata: &model.Metadata{
					Annotations:       []string{"annotation:my-annotation"},
					CreationTimestamp: creationTime.Unix(),
					Labels:            []string{"controller-uid:43739057-c6d7-4a5e-ab63-d0c8844e5272"},
					Name:              "job",
					Namespace:         "project",
					OwnerReferences: []*model.OwnerReference{
						{
							Kind: "CronJob",
							Name: "test-job",
							Uid:  "d0326ca4-d405-4fe9-99b5-7bfc4a6722b6",
						},
					},
					ResourceVersion: "220021511",
					Uid:             "8893e7a0-fc49-4627-b695-3ed47074ecba",
				},
				Spec: &model.JobSpec{
					BackoffLimit: 6,
					Completions:  1,
					Parallelism:  1,
					Selectors: []*model.LabelSelectorRequirement{
						{
							Key:      "controller-uid",
							Operator: "In",
							Values:   []string{"43739057-c6d7-4a5e-ab63-d0c8844e5272"},
						},
					},
				},
				Status: &model.JobStatus{
					CompletionTime: completionTime.Unix(),
					Succeeded:      1,
					StartTime:      startTime.Unix(),
				},
				Conditions: []*model.JobCondition{
					{
						LastProbeTime:      lastTransitionTime.Unix(),
						LastTransitionTime: lastTransitionTime.Unix(),
						Status:             string(corev1.ConditionTrue),
						Type:               string(batchv1.JobComplete),
					},
				},
				Tags: []string{"kube_condition_complete:true"},
			},
		},
		"job started by cronjob (failed)": {
			input: batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"annotation": "my-annotation",
					},
					CreationTimestamp: creationTime,
					Labels: map[string]string{
						"controller-uid": "43739057-c6d7-4a5e-ab63-d0c8844e5272",
					},
					Name:      "job",
					Namespace: "project",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "batch/v1beta1",
							Controller: pointer.Ptr(true),
							Kind:       "CronJob",
							Name:       "test-job",
							UID:        "d0326ca4-d405-4fe9-99b5-7bfc4a6722b6",
						},
					},
					ResourceVersion: "220021511",
					UID:             types.UID("8893e7a0-fc49-4627-b695-3ed47074ecba"),
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: pointer.Ptr(int32(6)),
					Completions:  pointer.Ptr(int32(1)),
					Parallelism:  pointer.Ptr(int32(1)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"controller-uid": "43739057-c6d7-4a5e-ab63-d0c8844e5272",
						},
					},
				},
				Status: batchv1.JobStatus{
					Failed: 1,
					Conditions: []batchv1.JobCondition{
						{
							LastProbeTime:      lastTransitionTime,
							LastTransitionTime: lastTransitionTime,
							Message:            "Job has reached the specified backoff limit",
							Reason:             "BackoffLimitExceeded",
							Status:             corev1.ConditionTrue,
							Type:               batchv1.JobFailed,
						},
					},
					StartTime: &startTime,
				},
			},
			expected: model.Job{
				Metadata: &model.Metadata{
					Annotations:       []string{"annotation:my-annotation"},
					CreationTimestamp: creationTime.Unix(),
					Labels:            []string{"controller-uid:43739057-c6d7-4a5e-ab63-d0c8844e5272"},
					Name:              "job",
					Namespace:         "project",
					OwnerReferences: []*model.OwnerReference{
						{
							Kind: "CronJob",
							Name: "test-job",
							Uid:  "d0326ca4-d405-4fe9-99b5-7bfc4a6722b6",
						},
					},
					ResourceVersion: "220021511",
					Uid:             "8893e7a0-fc49-4627-b695-3ed47074ecba",
				},
				Spec: &model.JobSpec{
					BackoffLimit: 6,
					Completions:  1,
					Parallelism:  1,
					Selectors: []*model.LabelSelectorRequirement{
						{
							Key:      "controller-uid",
							Operator: "In",
							Values:   []string{"43739057-c6d7-4a5e-ab63-d0c8844e5272"},
						},
					},
				},
				Status: &model.JobStatus{
					ConditionMessage: "Job has reached the specified backoff limit",
					Failed:           1,
					StartTime:        startTime.Unix(),
				},
				Conditions: []*model.JobCondition{
					{
						LastProbeTime:      lastTransitionTime.Unix(),
						LastTransitionTime: lastTransitionTime.Unix(),
						Message:            "Job has reached the specified backoff limit",
						Reason:             "BackoffLimitExceeded",
						Status:             string(corev1.ConditionTrue),
						Type:               string(batchv1.JobFailed),
					},
				},
				Tags: []string{"kube_condition_failed:true"},
			},
		},
		"job with resources": {
			input: batchv1.Job{
				Spec: batchv1.JobSpec{
					Template: getTemplateWithResourceRequirements(),
				},
			},
			expected: model.Job{
				Metadata: &model.Metadata{},
				Spec: &model.JobSpec{
					ResourceRequirements: getExpectedModelResourceRequirements(),
				},
				Status: &model.JobStatus{},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			pctx := &processors.K8sProcessorContext{
				LabelsAsTags:      tc.labelsAsTags,
				AnnotationsAsTags: tc.annotationsAsTags,
			}
			actual := ExtractJob(pctx, &tc.input)
			sort.Strings(actual.Tags)
			sort.Strings(tc.expected.Tags)
			sort.Strings(actual.Metadata.Labels)
			sort.Strings(tc.expected.Metadata.Labels)
			assert.Equal(t, &tc.expected, actual)
		})
	}
}
