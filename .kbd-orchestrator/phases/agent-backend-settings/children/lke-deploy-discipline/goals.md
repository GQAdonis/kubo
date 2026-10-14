# Goals — agent-backend-settings› lke-deploy-discipline

- Inventory live know-me-cluster conventions read-only (DNS providers per domain, issuers and solver attachment, gateways/listeners/LoadBalancer ports, Secret naming, Argo sync policy, restart rules) and the #24-#27 failures
- Write an agentskills.io-compliant skill lke-gitops-change in the know-me-cluster repo with a read-only live preflight script and generated cluster-facts reference, validated
- Prove the skill on pending know-me-cluster#27 and the OG-6 restart before either proceeds; later cluster changes must use it
