// Copyright 2018-2019 Authors of Cilium
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build !privileged_tests

package policy

import (
	"fmt"
	"sync"

	"github.com/cilium/cilium/pkg/checker"
	"github.com/cilium/cilium/pkg/identity"
	"github.com/cilium/cilium/pkg/identity/cache"
	"github.com/cilium/cilium/pkg/labels"
	"github.com/cilium/cilium/pkg/option"
	"github.com/cilium/cilium/pkg/policy/api"
	"github.com/cilium/cilium/pkg/policy/trafficdirection"
	. "gopkg.in/check.v1"
)

var (
	fooLabel = labels.NewLabel("k8s:foo", "", "")
	lbls     = labels.Labels{
		"foo": fooLabel,
	}
	lblsArray   = lbls.LabelArray()
	fooIdentity = &identity.Identity{
		ID:         303,
		Labels:     lbls,
		LabelArray: lbls.LabelArray(),
	}
	identityCache = cache.IdentityCache{303: lblsArray}
)

type dummyEndpoint struct {
	ID               uint16
	SecurityIdentity *identity.Identity
}

func (d *dummyEndpoint) GetID16() uint16 {
	return d.ID
}

func (d *dummyEndpoint) GetSecurityIdentity() (*identity.Identity, error) {
	return d.SecurityIdentity, nil
}

func (d *dummyEndpoint) PolicyRevisionBumpEvent(rev uint64) {
}

func GenerateNumIdentities(numIdentities int) {
	for i := 0; i < numIdentities; i++ {

		identityLabel := labels.NewLabel(fmt.Sprintf("k8s:foo%d", i), "", "")
		clusterLabel := labels.NewLabel("io.cilium.k8s.policy.cluster=default", "", labels.LabelSourceK8s)
		serviceAccountLabel := labels.NewLabel("io.cilium.k8s.policy.serviceaccount=default", "", labels.LabelSourceK8s)
		namespaceLabel := labels.NewLabel("io.kubernetes.pod.namespace=monitoring", "", labels.LabelSourceK8s)
		funLabel := labels.NewLabel("app=analytics-erneh", "", labels.LabelSourceK8s)

		identityLabels := labels.Labels{
			fmt.Sprintf("foo%d", i):                           identityLabel,
			"k8s:io.cilium.k8s.policy.cluster=default":        clusterLabel,
			"k8s:io.cilium.k8s.policy.serviceaccount=default": serviceAccountLabel,
			"k8s:io.kubernetes.pod.namespace=monitoring":      namespaceLabel,
			"k8s:app=analytics-erneh":                         funLabel,
		}

		bumpedIdentity := i + 1000
		numericIdentity := identity.NumericIdentity(bumpedIdentity)

		identityCache[numericIdentity] = identityLabels.LabelArray()
	}
}

func GenerateL3IngressRules(numRules int) api.Rules {
	parseFooLabel := labels.ParseSelectLabel("k8s:foo")
	fooSelector := api.NewESFromLabels(parseFooLabel)
	barSelector := api.NewESFromLabels(labels.ParseSelectLabel("bar"))

	// Change ingRule and rule in the for-loop below to change what type of rules
	// are added into the policy repository.
	ingRule := api.IngressRule{
		FromEndpoints: []api.EndpointSelector{barSelector},
	}

	var rules api.Rules
	for i := 1; i <= numRules; i++ {

		rule := api.Rule{
			EndpointSelector: fooSelector,
			Ingress:          []api.IngressRule{ingRule},
		}
		rule.Sanitize()
		rules = append(rules, &rule)
	}
	return rules
}

func GenerateL3EgressRules(numRules int) api.Rules {
	parseFooLabel := labels.ParseSelectLabel("k8s:foo")
	fooSelector := api.NewESFromLabels(parseFooLabel)
	barSelector := api.NewESFromLabels(labels.ParseSelectLabel("bar"))

	// Change ingRule and rule in the for-loop below to change what type of rules
	// are added into the policy repository.
	egRule := api.EgressRule{
		ToEndpoints: []api.EndpointSelector{barSelector},
	}

	var rules api.Rules
	for i := 1; i <= numRules; i++ {

		rule := api.Rule{
			EndpointSelector: fooSelector,
			Egress:           []api.EgressRule{egRule},
		}
		rule.Sanitize()
		rules = append(rules, &rule)
	}
	return rules
}

func GenerateCIDRRules(numRules int) api.Rules {
	parseFooLabel := labels.ParseSelectLabel("k8s:foo")
	fooSelector := api.NewESFromLabels(parseFooLabel)
	//barSelector := api.NewESFromLabels(labels.ParseSelectLabel("bar"))

	// Change ingRule and rule in the for-loop below to change what type of rules
	// are added into the policy repository.
	egRule := api.EgressRule{
		ToCIDR: []api.CIDR{api.CIDR("10.2.3.0/24"), api.CIDR("ff02::/64")},
		/*ToRequires:  []api.EndpointSelector{barSelector},
		ToPorts: []api.PortRule{
			{
				Ports: []api.PortProtocol{
					{
						Port:     "8080",
						Protocol: api.ProtoTCP,
					},
				},
			},
		},*/
	}

	var rules api.Rules
	for i := 1; i <= numRules; i++ {

		rule := api.Rule{
			EndpointSelector: fooSelector,
			Egress:           []api.EgressRule{egRule},
		}
		rule.Sanitize()
		rules = append(rules, &rule)
	}
	return rules
}

type DummyOwner struct{}

func (d DummyOwner) LookupRedirectPort(l4 *L4Filter) uint16 {
	return 0
}

func bootstrapRepo(ruleGenFunc func(int) api.Rules, numRules int, c *C) *Repository {
	testRepo := NewPolicyRepository()

	var wg sync.WaitGroup
	SetPolicyEnabled(option.DefaultEnforcement)
	GenerateNumIdentities(3000)
	testSelectorCache.UpdateIdentities(identityCache, nil)
	testRepo.selectorCache = testSelectorCache
	rulez, _ := testRepo.AddList(ruleGenFunc(numRules))

	epSet := NewEndpointSet(map[Endpoint]struct{}{
		&dummyEndpoint{
			ID:               9001,
			SecurityIdentity: fooIdentity,
		}: {},
	})

	epsToRegen := NewEndpointSet(nil)
	rulez.UpdateRulesEndpointsCaches(epSet, epsToRegen, &wg)
	wg.Wait()

	c.Assert(epSet.Len(), Equals, 0)
	c.Assert(epsToRegen.Len(), Equals, 1)

	return testRepo
}

func (ds *PolicyTestSuite) BenchmarkRegenerateCIDRPolicyRules(c *C) {
	testRepo := bootstrapRepo(GenerateCIDRRules, 1000, c)

	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		ip, _ := testRepo.resolvePolicyLocked(fooIdentity)
		_ = ip.DistillPolicy(DummyOwner{})
		ip.Detach()
	}
}

func (ds *PolicyTestSuite) BenchmarkRegenerateL3IngressPolicyRules(c *C) {
	testRepo := bootstrapRepo(GenerateL3IngressRules, 1000, c)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		ip, _ := testRepo.resolvePolicyLocked(fooIdentity)
		_ = ip.DistillPolicy(DummyOwner{})
		ip.Detach()
	}
}

func (ds *PolicyTestSuite) BenchmarkRegenerateL3EgressPolicyRules(c *C) {
	testRepo := bootstrapRepo(GenerateL3EgressRules, 1000, c)
	c.ResetTimer()
	for i := 0; i < c.N; i++ {
		ip, _ := testRepo.resolvePolicyLocked(fooIdentity)
		_ = ip.DistillPolicy(DummyOwner{})
		ip.Detach()
	}
}

func (ds *PolicyTestSuite) TestL7WithIngressWildcard(c *C) {
	repo := bootstrapRepo(GenerateL3IngressRules, 1000, c)

	idFooSelectLabelArray := labels.ParseSelectLabelArray("id=foo")
	idFooSelectLabels := labels.Labels{}
	for _, lbl := range idFooSelectLabelArray {
		idFooSelectLabels[lbl.Key] = lbl
	}
	fooIdentity := identity.NewIdentity(12345, idFooSelectLabels)

	selFoo := api.NewESFromLabels(labels.ParseSelectLabel("id=foo"))
	rule1 := api.Rule{
		EndpointSelector: selFoo,
		Ingress: []api.IngressRule{
			{
				ToPorts: []api.PortRule{{
					Ports: []api.PortProtocol{
						{Port: "80", Protocol: api.ProtoTCP},
					},
					Rules: &api.L7Rules{
						HTTP: []api.PortRuleHTTP{
							{Method: "GET", Path: "/good"},
						},
					},
				}},
			},
		},
	}

	rule1.Sanitize()
	_, _, err := repo.Add(rule1, []Endpoint{})
	c.Assert(err, IsNil)

	repo.Mutex.RLock()
	defer repo.Mutex.RUnlock()
	selPolicy, err := repo.resolvePolicyLocked(fooIdentity)
	c.Assert(err, IsNil)
	policy := selPolicy.DistillPolicy(DummyOwner{})

	expectedEndpointPolicy := EndpointPolicy{
		selectorPolicy: &selectorPolicy{
			Revision:      repo.GetRevision(),
			SelectorCache: repo.GetSelectorCache(),
			L4Policy: &L4Policy{
				Revision: repo.GetRevision(),
				Ingress: L4PolicyMap{
					"80/TCP": {
						Port:     80,
						Protocol: api.ProtoTCP,
						U8Proto:  0x6,
						CachedSelectors: CachedSelectorSlice{
							wildcardCachedSelector,
						},
						allowsAllAtL3: true,
						L7Parser:      ParserTypeHTTP,
						Ingress:       true,
						L7RulesPerEp: L7DataMap{
							wildcardCachedSelector: api.L7Rules{
								HTTP: []api.PortRuleHTTP{{Method: "GET", Path: "/good"}},
							},
						},
						DerivedFromRules: labels.LabelArrayList{nil},
					},
				},
				Egress: L4PolicyMap{},
			},
			CIDRPolicy:           policy.CIDRPolicy,
			IngressPolicyEnabled: true,
			EgressPolicyEnabled:  false,
		},
		PolicyOwner: DummyOwner{},
		// inherit this from the result as it is outside of the scope
		// of this test
		PolicyMapState: policy.PolicyMapState,
	}

	// Have to remove circular reference before testing to avoid an infinite loop
	policy.selectorPolicy.Detach()

	c.Assert(policy, checker.Equals, &expectedEndpointPolicy)
}

func (ds *PolicyTestSuite) TestL7WithLocalHostWildcardd(c *C) {
	repo := bootstrapRepo(GenerateL3IngressRules, 1000, c)

	idFooSelectLabelArray := labels.ParseSelectLabelArray("id=foo")
	idFooSelectLabels := labels.Labels{}
	for _, lbl := range idFooSelectLabelArray {
		idFooSelectLabels[lbl.Key] = lbl
	}

	fooIdentity := identity.NewIdentity(12345, idFooSelectLabels)

	// Emulate Kubernetes mode with allow from localhost
	oldLocalhostOpt := option.Config.AllowLocalhost
	option.Config.AllowLocalhost = option.AllowLocalhostAlways
	defer func() { option.Config.AllowLocalhost = oldLocalhostOpt }()

	selFoo := api.NewESFromLabels(labels.ParseSelectLabel("id=foo"))
	rule1 := api.Rule{
		EndpointSelector: selFoo,
		Ingress: []api.IngressRule{
			{
				ToPorts: []api.PortRule{{
					Ports: []api.PortProtocol{
						{Port: "80", Protocol: api.ProtoTCP},
					},
					Rules: &api.L7Rules{
						HTTP: []api.PortRuleHTTP{
							{Method: "GET", Path: "/good"},
						},
					},
				}},
			},
		},
	}

	rule1.Sanitize()
	_, _, err := repo.Add(rule1, []Endpoint{})
	c.Assert(err, IsNil)

	repo.Mutex.RLock()
	defer repo.Mutex.RUnlock()

	selPolicy, err := repo.resolvePolicyLocked(fooIdentity)
	c.Assert(err, IsNil)
	policy := selPolicy.DistillPolicy(DummyOwner{})

	cachedSelectorHost := testSelectorCache.FindCachedIdentitySelector(api.ReservedEndpointSelectors[labels.IDNameHost])
	c.Assert(cachedSelectorHost, Not(IsNil))

	expectedEndpointPolicy := EndpointPolicy{
		selectorPolicy: &selectorPolicy{
			Revision:      repo.GetRevision(),
			SelectorCache: repo.GetSelectorCache(),
			L4Policy: &L4Policy{
				Revision: repo.GetRevision(),
				Ingress: L4PolicyMap{
					"80/TCP": {
						Port:     80,
						Protocol: api.ProtoTCP,
						U8Proto:  0x6,
						CachedSelectors: CachedSelectorSlice{
							wildcardCachedSelector,
							cachedSelectorHost,
						},
						allowsAllAtL3: true,
						L7Parser:      ParserTypeHTTP,
						Ingress:       true,
						L7RulesPerEp: L7DataMap{
							wildcardCachedSelector: api.L7Rules{
								HTTP: []api.PortRuleHTTP{{Method: "GET", Path: "/good"}},
							},
							cachedSelectorHost: api.L7Rules{},
						},
						DerivedFromRules: labels.LabelArrayList{nil},
					},
				},
				Egress: L4PolicyMap{},
			},
			CIDRPolicy:           policy.CIDRPolicy,
			IngressPolicyEnabled: true,
			EgressPolicyEnabled:  false,
		},
		PolicyOwner: DummyOwner{},
		// inherit this from the result as it is outside of the scope
		// of this test
		PolicyMapState: policy.PolicyMapState,
	}

	// Have to remove circular reference before testing to avoid an infinite loop
	policy.selectorPolicy.Detach()

	c.Assert(policy, checker.Equals, &expectedEndpointPolicy)
}

func (ds *PolicyTestSuite) TestMapStateWithIngressWildcard(c *C) {
	repo := bootstrapRepo(GenerateL3IngressRules, 1000, c)

	idFooSelectLabelArray := labels.ParseSelectLabelArray("id=foo")
	idFooSelectLabels := labels.Labels{}
	for _, lbl := range idFooSelectLabelArray {
		idFooSelectLabels[lbl.Key] = lbl
	}
	fooIdentity := identity.NewIdentity(12345, idFooSelectLabels)

	selFoo := api.NewESFromLabels(labels.ParseSelectLabel("id=foo"))
	rule1 := api.Rule{
		EndpointSelector: selFoo,
		Ingress: []api.IngressRule{
			{
				ToPorts: []api.PortRule{{
					Ports: []api.PortProtocol{
						{Port: "80", Protocol: api.ProtoTCP},
					},
					Rules: &api.L7Rules{},
				}},
			},
		},
	}

	rule1.Sanitize()
	_, _, err := repo.Add(rule1, []Endpoint{})
	c.Assert(err, IsNil)

	repo.Mutex.RLock()
	defer repo.Mutex.RUnlock()
	selPolicy, err := repo.resolvePolicyLocked(fooIdentity)
	c.Assert(err, IsNil)
	policy := selPolicy.DistillPolicy(DummyOwner{})

	expectedEndpointPolicy := EndpointPolicy{
		selectorPolicy: &selectorPolicy{
			Revision:      repo.GetRevision(),
			SelectorCache: repo.GetSelectorCache(),
			L4Policy: &L4Policy{
				Revision: repo.GetRevision(),
				Ingress: L4PolicyMap{
					"80/TCP": {
						Port:     80,
						Protocol: api.ProtoTCP,
						U8Proto:  0x6,
						CachedSelectors: CachedSelectorSlice{
							wildcardCachedSelector,
						},
						allowsAllAtL3:    true,
						L7Parser:         ParserTypeNone,
						Ingress:          true,
						L7RulesPerEp:     L7DataMap{},
						DerivedFromRules: labels.LabelArrayList{nil},
					},
				},
				Egress: L4PolicyMap{},
			},
			CIDRPolicy:           policy.CIDRPolicy,
			IngressPolicyEnabled: true,
			EgressPolicyEnabled:  false,
		},
		PolicyOwner: DummyOwner{},
		PolicyMapState: MapState{
			{TrafficDirection: trafficdirection.Egress.Uint8()}: {},
			{DestPort: 80, Nexthdr: 6}:                          {},
		},
	}

	// Add new identity to test accumulation of PolicyMapChanges
	added1 := cache.IdentityCache{
		identity.NumericIdentity(192): labels.ParseSelectLabelArray("id=resolve_test_1"),
	}
	testSelectorCache.UpdateIdentities(added1, nil)
	c.Assert(policy.PolicyMapChanges.adds, HasLen, 0)
	c.Assert(policy.PolicyMapChanges.deletes, HasLen, 0)

	// Have to remove circular reference before testing to avoid an infinite loop
	policy.selectorPolicy.Detach()

	c.Assert(policy, checker.Equals, &expectedEndpointPolicy)
}

func (ds *PolicyTestSuite) TestMapStateWithIngress(c *C) {
	repo := bootstrapRepo(GenerateL3IngressRules, 1000, c)

	idFooSelectLabelArray := labels.ParseSelectLabelArray("id=foo")
	idFooSelectLabels := labels.Labels{}
	for _, lbl := range idFooSelectLabelArray {
		idFooSelectLabels[lbl.Key] = lbl
	}
	fooIdentity := identity.NewIdentity(12345, idFooSelectLabels)

	lblTest := labels.ParseLabel("id=resolve_test_1")

	selFoo := api.NewESFromLabels(labels.ParseSelectLabel("id=foo"))
	rule1 := api.Rule{
		EndpointSelector: selFoo,
		Ingress: []api.IngressRule{
			{
				FromEntities: []api.Entity{api.EntityWorld},
				ToPorts: []api.PortRule{{
					Ports: []api.PortProtocol{
						{Port: "80", Protocol: api.ProtoTCP},
					},
					Rules: &api.L7Rules{},
				}},
			},
			{
				FromEndpoints: []api.EndpointSelector{
					api.NewESFromLabels(lblTest),
				},
				ToPorts: []api.PortRule{{
					Ports: []api.PortProtocol{
						{Port: "80", Protocol: api.ProtoTCP},
					},
					Rules: &api.L7Rules{},
				}},
			},
		},
	}

	rule1.Sanitize()
	_, _, err := repo.Add(rule1, []Endpoint{})
	c.Assert(err, IsNil)

	repo.Mutex.RLock()
	defer repo.Mutex.RUnlock()
	selPolicy, err := repo.resolvePolicyLocked(fooIdentity)
	c.Assert(err, IsNil)
	policy := selPolicy.DistillPolicy(DummyOwner{})

	// Add new identity to test accumulation of PolicyMapChanges
	added1 := cache.IdentityCache{
		identity.NumericIdentity(192): labels.ParseSelectLabelArray("id=resolve_test_1", "num=1"),
		identity.NumericIdentity(193): labels.ParseSelectLabelArray("id=resolve_test_1", "num=2"),
		identity.NumericIdentity(194): labels.ParseSelectLabelArray("id=resolve_test_1", "num=3"),
	}
	testSelectorCache.UpdateIdentities(added1, nil)
	c.Assert(policy.PolicyMapChanges.adds, HasLen, 3)
	c.Assert(policy.PolicyMapChanges.deletes, HasLen, 0)

	deleted1 := cache.IdentityCache{
		identity.NumericIdentity(193): labels.ParseSelectLabelArray("id=resolve_test_1", "num=2"),
	}
	testSelectorCache.UpdateIdentities(nil, deleted1)
	c.Assert(policy.PolicyMapChanges.adds, HasLen, 2)
	c.Assert(policy.PolicyMapChanges.deletes, HasLen, 1)

	cachedSelectorWorld := testSelectorCache.FindCachedIdentitySelector(api.ReservedEndpointSelectors[labels.IDNameWorld])
	c.Assert(cachedSelectorWorld, Not(IsNil))

	cachedSelectorTest := testSelectorCache.FindCachedIdentitySelector(api.NewESFromLabels(lblTest))
	c.Assert(cachedSelectorTest, Not(IsNil))

	expectedEndpointPolicy := EndpointPolicy{
		selectorPolicy: &selectorPolicy{
			Revision:      repo.GetRevision(),
			SelectorCache: repo.GetSelectorCache(),
			L4Policy: &L4Policy{
				Revision: repo.GetRevision(),
				Ingress: L4PolicyMap{
					"80/TCP": {
						Port:     80,
						Protocol: api.ProtoTCP,
						U8Proto:  0x6,
						CachedSelectors: CachedSelectorSlice{
							cachedSelectorWorld,
							cachedSelectorTest,
						},
						allowsAllAtL3:    false,
						L7Parser:         ParserTypeNone,
						Ingress:          true,
						L7RulesPerEp:     L7DataMap{},
						DerivedFromRules: labels.LabelArrayList{nil, nil},
					},
				},
				Egress: L4PolicyMap{},
			},
			CIDRPolicy:           policy.CIDRPolicy,
			IngressPolicyEnabled: true,
			EgressPolicyEnabled:  false,
		},
		PolicyOwner: DummyOwner{},
		PolicyMapState: MapState{
			{TrafficDirection: trafficdirection.Egress.Uint8()}:                          {},
			{Identity: uint32(identity.ReservedIdentityWorld), DestPort: 80, Nexthdr: 6}: {},
		},
		PolicyMapChanges: MapChanges{
			adds: MapState{
				{Identity: 192, DestPort: 80, Nexthdr: 6}: {},
				{Identity: 194, DestPort: 80, Nexthdr: 6}: {},
			},
			deletes: MapState{
				{Identity: 193, DestPort: 80, Nexthdr: 6}: {},
			},
		},
	}

	// Have to remove circular reference before testing for Equality to avoid an infinite loop
	policy.selectorPolicy.Detach()
	// Verify that cached selector is not found after Detach().
	// Note that this depends on the other tests NOT using the same selector concurrently!
	cachedSelectorTest = testSelectorCache.FindCachedIdentitySelector(api.NewESFromLabels(lblTest))
	c.Assert(cachedSelectorTest, IsNil)

	c.Assert(policy, checker.Equals, &expectedEndpointPolicy)

	adds, deletes := policy.PolicyMapChanges.ConsumeMapChanges()
	// maps on the policy got cleared
	c.Assert(policy.PolicyMapChanges.adds, IsNil)
	c.Assert(policy.PolicyMapChanges.deletes, IsNil)

	c.Assert(adds, checker.Equals, MapState{
		{Identity: 192, DestPort: 80, Nexthdr: 6}: {},
		{Identity: 194, DestPort: 80, Nexthdr: 6}: {},
	})
	c.Assert(deletes, checker.Equals, MapState{
		{Identity: 193, DestPort: 80, Nexthdr: 6}: {},
	})
}
