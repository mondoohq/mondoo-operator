/*
Copyright 2022 Mondoo, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodes

import (
	"context"
	"os"
	"regexp"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// irsaRoleAnnotation is the annotation IRSA uses to bind a ServiceAccount to an
// IAM role.
const irsaRoleAnnotation = "eks.amazonaws.com/role-arn"

// stsTimeout bounds the GetCallerIdentity call. The result is cached for the
// life of the process, so this runs at most once; a cluster with no route to
// STS should not stall a reconcile waiting for it.
const stsTimeout = 5 * time.Second

// iamRoleARNRegex extracts the account ID from an IAM role ARN, e.g.
// arn:aws:iam::123456789012:role/some-role. It accepts every partition, so
// aws-cn and aws-us-gov ARNs work too.
var iamRoleARNRegex = regexp.MustCompile(`^arn:aws[a-z-]*:iam::(\d{12}):`)

// awsAccountResolver finds the AWS account the cluster runs in, without going
// through the instance metadata service.
//
// IMDS is the obvious source and the wrong one here: Karpenter and the EKS
// managed node group module both default httpPutResponseHopLimit to 1, which
// stops any pod that is not on the host network from reading it. Raising that
// limit exposes the node role to every pod on the node, and host networking is
// a heavy grant for a scan pod. Every source below avoids the problem instead.
//
// The account never changes for a given operator deployment, so the answer is
// resolved once and cached -- including a negative answer, which stops a
// cluster with no AWS identity from retrying on every reconcile.
type awsAccountResolver struct {
	// Namespace is the operator's own namespace, where the ServiceAccount
	// fallback looks for IRSA annotations.
	Namespace string

	once    sync.Once
	account string
}

// awsAccountResolvers caches one resolver per operator namespace. The account
// is a property of the cluster, so the answer is worth keeping across
// reconciles -- the DeploymentHandler that asks for it is rebuilt on every one.
// Keyed by namespace rather than held as a single global because the fallback
// branch looks for ServiceAccounts in a specific namespace, and an operator may
// reconcile MondooAuditConfigs in more than one.
var awsAccountResolvers sync.Map

// awsAccountResolverFor returns the cached resolver for an operator namespace.
func awsAccountResolverFor(namespace string) *awsAccountResolver {
	if resolver, ok := awsAccountResolvers.Load(namespace); ok {
		return resolver.(*awsAccountResolver)
	}
	resolver, _ := awsAccountResolvers.LoadOrStore(namespace, &awsAccountResolver{Namespace: namespace})
	return resolver.(*awsAccountResolver)
}

// Resolve returns the AWS account ID, or "" when it cannot be established. An
// empty result is a normal outcome -- a cluster outside AWS, or one whose
// operator has no cloud identity -- and callers must treat it as "skip the AWS
// platform ID", never as an error worth failing a reconcile over.
func (r *awsAccountResolver) Resolve(ctx context.Context, c client.Client) string {
	r.once.Do(func() {
		r.account = r.resolve(ctx, c)
	})
	return r.account
}

func (r *awsAccountResolver) resolve(ctx context.Context, c client.Client) string {
	// 1. IRSA injects AWS_ROLE_ARN into every pod it serves, so the account is
	//    already in this process's environment. Free, and no network call.
	if account := accountFromRoleARN(os.Getenv("AWS_ROLE_ARN")); account != "" {
		return account
	}

	// 2. Ask STS. This is the authoritative answer and the only one that works
	//    under EKS Pod Identity, which hands out credentials without ever
	//    naming a role ARN in the environment. GetCallerIdentity needs no IAM
	//    permission at all -- any valid credentials can call it -- so this adds
	//    no privilege requirement, only the need for credentials to exist.
	if account := accountFromSTS(ctx); account != "" {
		return account
	}

	// 3. Fall back to reading an IRSA annotation off a ServiceAccount in the
	//    operator's namespace. This needs no AWS credentials at all, which
	//    makes it the branch that works on a deployment where only the
	//    scanning ServiceAccount was ever wired up to IAM.
	//
	//    It is last because it is the weakest: it reports the account of
	//    whatever role somebody annotated, which is the cluster's account in
	//    every ordinary setup -- the OIDC provider backing IRSA lives there --
	//    but would be wrong for a role assumed across accounts.
	return r.accountFromServiceAccounts(ctx, c)
}

// accountFromRoleARN pulls the account ID out of an IAM role ARN.
func accountFromRoleARN(arn string) string {
	match := iamRoleARNRegex.FindStringSubmatch(arn)
	if match == nil {
		return ""
	}
	return match[1]
}

// accountFromSTS asks STS who we are. Returns "" whenever no usable credentials
// are configured, which is the common case outside AWS.
func accountFromSTS(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, stsTimeout)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		logger.V(1).Info("could not load an AWS configuration; skipping STS", "error", err.Error())
		return ""
	}

	out, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		logger.V(1).Info("could not resolve the AWS account from STS", "error", err.Error())
		return ""
	}
	if out.Account == nil || !awsAccountIDRegex.MatchString(*out.Account) {
		return ""
	}
	return *out.Account
}

// accountFromServiceAccounts scans the operator's namespace for an IRSA
// annotation and takes the account from the role ARN it points at.
func (r *awsAccountResolver) accountFromServiceAccounts(ctx context.Context, c client.Client) string {
	if c == nil || r.Namespace == "" {
		return ""
	}

	serviceAccounts := &corev1.ServiceAccountList{}
	if err := c.List(ctx, serviceAccounts, client.InNamespace(r.Namespace)); err != nil {
		logger.V(1).Info("could not list service accounts for AWS account detection", "error", err.Error())
		return ""
	}

	for _, sa := range serviceAccounts.Items {
		if account := accountFromRoleARN(sa.Annotations[irsaRoleAnnotation]); account != "" {
			return account
		}
	}
	return ""
}
