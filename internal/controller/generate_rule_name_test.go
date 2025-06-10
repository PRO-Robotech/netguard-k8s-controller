package controller

//
//func TestGenerateRuleName(t *testing.T) {
//	reconciler := &RuleS2SReconciler{}
//
//	t.Run("ConsistentNames", func(t *testing.T) {
//		// Generate name twice with the same input
//		name1 := reconciler.generateRuleName("test-rule", "ingress", "local-ag", "target-ag", "TCP")
//		name2 := reconciler.generateRuleName("test-rule", "ingress", "local-ag", "target-ag", "TCP")
//
//		// Names should be identical
//		if name1 != name2 {
//			t.Errorf("Expected consistent names, got %s and %s", name1, name2)
//		}
//	})
//
//	t.Run("DifferentTrafficDirections", func(t *testing.T) {
//		// Generate names for ingress and egress
//		ingressName := reconciler.generateRuleName("test-rule", "ingress", "local-ag", "target-ag", "TCP")
//		egressName := reconciler.generateRuleName("test-rule", "egress", "local-ag", "target-ag", "TCP")
//
//		// Names should be different
//		if ingressName == egressName {
//			t.Errorf("Expected different names for different traffic directions, got %s for both", ingressName)
//		}
//
//		// Ingress name should start with "ing-"
//		if ingressName[:4] != "ing-" {
//			t.Errorf("Expected ingress name to start with 'ing-', got %s", ingressName)
//		}
//
//		// Egress name should start with "egr-"
//		if egressName[:4] != "egr-" {
//			t.Errorf("Expected egress name to start with 'egr-', got %s", egressName)
//		}
//	})
//
//	t.Run("DifferentProtocols", func(t *testing.T) {
//		// Generate names for TCP and UDP
//		tcpName := reconciler.generateRuleName("test-rule", "ingress", "local-ag", "target-ag", "TCP")
//		udpName := reconciler.generateRuleName("test-rule", "ingress", "local-ag", "target-ag", "UDP")
//
//		// Names should be different
//		if tcpName == udpName {
//			t.Errorf("Expected different names for different protocols, got %s for both", tcpName)
//		}
//	})
//
//	t.Run("LongAddressGroupNames", func(t *testing.T) {
//		// Create very long address group names (over 63 characters)
//		longLocalAGName := "very-long-local-address-group-name-that-exceeds-kubernetes-name-limit"
//		longTargetAGName := "very-long-target-address-group-name-that-exceeds-kubernetes-name-limit"
//
//		// Generate name with long address group names
//		name := reconciler.generateRuleName("test-rule", "ingress", longLocalAGName, longTargetAGName, "TCP")
//
//		// Name should be 63 characters or less (Kubernetes name limit)
//		if len(name) > 63 {
//			t.Errorf("Expected name length to be <= 63, got %d: %s", len(name), name)
//		}
//	})
//
//	t.Run("DifferentInputs", func(t *testing.T) {
//		// Generate names with different inputs
//		name1 := reconciler.generateRuleName("rule1", "ingress", "local-ag", "target-ag", "TCP")
//		name2 := reconciler.generateRuleName("rule2", "ingress", "local-ag", "target-ag", "TCP")
//		name3 := reconciler.generateRuleName("rule1", "ingress", "different-local-ag", "target-ag", "TCP")
//		name4 := reconciler.generateRuleName("rule1", "ingress", "local-ag", "different-target-ag", "TCP")
//
//		// All names should be different
//		if name1 == name2 || name1 == name3 || name1 == name4 || name2 == name3 || name2 == name4 || name3 == name4 {
//			t.Errorf("Expected different names for different inputs, got duplicates: %s, %s, %s, %s",
//				name1, name2, name3, name4)
//		}
//	})
//}
