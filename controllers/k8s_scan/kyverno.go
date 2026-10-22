// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"fmt"
	"slices"
	"strings"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

type invalidKyvernoConfigError struct{ err error }

func (e invalidKyvernoConfigError) Error() string { return e.err.Error() }
func (e invalidKyvernoConfigError) Unwrap() error { return e.err }

const (
	kyvernoDefaultMappings                   = "kyverno-default-mappings"
	kyvernoMappingAnnotationCheckUIDs        = "kyverno-mapping-annotation-check-uids"
	kyvernoMappingAnnotationCheckMRNs        = "kyverno-mapping-annotation-check-mrns"
	kyvernoMappingAnnotationPolicyUIDs       = "kyverno-mapping-annotation-policy-uids"
	kyvernoMappingAnnotationReasons          = "kyverno-mapping-annotation-reasons"
	kyvernoExceptionAnnotationValidUntil     = "kyverno-exception-annotation-valid-until"
	kyvernoExceptionAnnotationJustifications = "kyverno-exception-annotation-justifications"
	kyvernoExceptionAnnotationOwners         = "kyverno-exception-annotation-owners"
	kyvernoExceptionAnnotationTickets        = "kyverno-exception-annotation-tickets"
	kyvernoMirrorPolicyExceptions            = "kyverno-mirror-policy-exceptions"
	kyvernoMirroredExceptionApproval         = "kyverno-mirrored-exception-approval"
	kyvernoMirroredExceptionAction           = "kyverno-mirrored-exception-action"
	kyvernoFailExpiredPolicyExceptions       = "kyverno-fail-expired-policy-exceptions"
	kyvernoReportUnmappedPolicyExceptions    = "kyverno-report-unmapped-policy-exceptions"
	kyvernoReportUnmappedPolicyResults       = "kyverno-report-unmapped-policy-results"
)

func addKyvernoOptions(options map[string]string, spec v1alpha2.KyvernoSpec) error {
	if !spec.Enabled() {
		return nil
	}
	for option, values := range map[string][]string{
		kyvernoMappingAnnotationCheckUIDs:        spec.MappingAnnotations.CheckUIDs,
		kyvernoMappingAnnotationCheckMRNs:        spec.MappingAnnotations.CheckMRNs,
		kyvernoMappingAnnotationPolicyUIDs:       spec.MappingAnnotations.PolicyUIDs,
		kyvernoMappingAnnotationReasons:          spec.MappingAnnotations.Reasons,
		kyvernoExceptionAnnotationValidUntil:     spec.ExceptionAnnotations.ValidUntil,
		kyvernoExceptionAnnotationJustifications: spec.ExceptionAnnotations.Justifications,
		kyvernoExceptionAnnotationOwners:         spec.ExceptionAnnotations.Owners,
		kyvernoExceptionAnnotationTickets:        spec.ExceptionAnnotations.Tickets,
	} {
		for _, value := range values {
			if strings.Contains(value, ",") {
				return invalidKyvernoConfigError{err: fmt.Errorf("%s values must not contain commas: %q", option, value)}
			}
		}
		if len(values) > 0 {
			options[option] = strings.Join(values, ",")
		}
	}
	options[kyvernoDefaultMappings] = fmt.Sprintf("%t", spec.DefaultMappingsEnabled())
	options[kyvernoMirrorPolicyExceptions] = fmt.Sprintf("%t", spec.MirrorPolicyExceptionsEnabled())
	options[kyvernoMirroredExceptionApproval] = spec.MirroredExceptionApprovalMode()
	options[kyvernoMirroredExceptionAction] = spec.MirroredExceptionActionMode()
	options[kyvernoFailExpiredPolicyExceptions] = fmt.Sprintf("%t", spec.FailExpiredPolicyExceptionsEnabled())
	options[kyvernoReportUnmappedPolicyExceptions] = fmt.Sprintf("%t", spec.ReportUnmappedPolicyExceptionsEnabled())
	options[kyvernoReportUnmappedPolicyResults] = fmt.Sprintf("%t", spec.ReportUnmappedPolicyResultsEnabled())
	return nil
}

func kyvernoDiscoveryTargets(targets []string, spec v1alpha2.KyvernoSpec) []string {
	if !spec.Enabled() {
		return targets
	}
	if !slices.Contains(targets, "kyverno") {
		return append(targets, "kyverno")
	}
	return targets
}
