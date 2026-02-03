package refinement

var roleTemplates = map[string][]string{
	"frontend-vue": {
		"Vue Staff Engineer",
		"Design Systems Architect",
		"Frontend QA Specialist",
		"Performance Optimization Engineer",
		"Accessibility Lead",
		"Browser Compatibility Analyst",
		"Animations & Motion Reviewer",
		"Component Library Steward",
		"DX Coach",
		"DevOps Release Captain",
	},
	"frontend-react": {
		"React Staff Engineer",
		"Design Systems Architect",
		"State Management Reviewer",
		"Accessibility Champion",
		"SSR Performance Engineer",
		"QA Automation Lead",
		"Security Analyst",
		"UX Research Liaison",
		"Internationalization Reviewer",
		"Bundler Optimization Lead",
	},
	"frontend-svelte": {
		"Svelte Principal Engineer",
		"Compiler Performance Analyst",
		"Styling Consistency Reviewer",
		"Accessibility Lead",
		"Form Validation Expert",
		"QA Automation Lead",
		"DX Coach",
		"Animation Curator",
		"Internationalization Reviewer",
		"DevOps Captain",
	},
	"frontend-angular": {
		"Angular Architect",
		"Type Safety Reviewer",
		"Change Detection Specialist",
		"Accessibility Advocate",
		"Security Champion",
		"Testing & QA Lead",
		"Documentation Steward",
		"Performance Engineer",
		"Release Captain",
		"Observability Reviewer",
	},
	"node-service": {
		"Backend Staff Engineer",
		"API Contract Reviewer",
		"Database Reliability Lead",
		"Security Engineer",
		"Load Testing Lead",
		"Observability Advocate",
		"Incident Response Captain",
		"DevOps Release Lead",
		"Cost Optimization Analyst",
		"Integration QA Specialist",
	},
	"node-app": {
		"Node Platform Lead",
		"Full-stack QA Specialist",
		"Security Champion",
		"Performance Engineer",
		"API Contract Reviewer",
		"Documentation Steward",
		"Accessibility Advocate",
		"DevOps Captain",
		"Observability Lead",
		"Customer Support Liaison",
	},
	"go-service": {
		"Go Staff Engineer",
		"API Contract Reviewer",
		"Database Reliability Lead",
		"Security Engineer",
		"Load Testing Lead",
		"Observability Advocate",
		"Resilience Engineer",
		"Deployment Captain",
		"Cost Optimization Lead",
		"Integration QA",
	},
	"python-app": {
		"Python Staff Engineer",
		"Data Integrity Reviewer",
		"API Contract Lead",
		"Security Engineer",
		"QA Automation Lead",
		"Performance Tuning Specialist",
		"DevOps Release Captain",
		"Documentation Steward",
		"Customer Advocate",
		"Observability Lead",
	},
	"rust-app": {
		"Rust Systems Architect",
		"Memory Safety Reviewer",
		"Performance Engineer",
		"Security Analyst",
		"Testing & QA Lead",
		"DevOps Captain",
		"Tooling & DX Coach",
		"Documentation Steward",
		"Release Captain",
		"Platform Reliability Lead",
	},
}

var fallbackRoles = []string{
	"Staff Engineer",
	"QA Specialist",
	"Performance Engineer",
	"Security Analyst",
	"Customer Advocate",
	"Documentation Steward",
	"Observability Lead",
	"Integration Tester",
	"Release Captain",
	"Support Liaison",
	"UX Reviewer",
	"Accessibility Champion",
}

func generateRoles(profile projectProfile) []string {
	roles := append([]string{}, roleTemplates[profile.Type]...)
	if len(roles) < 10 {
		roles = append(roles, fallbackRoles...)
	}
	unique := dedupe(roles)
	if len(unique) < 10 {
		for i := len(unique); i < 10; i++ {
			unique = append(unique, fallbackRoles[i%len(fallbackRoles)])
		}
	}
	return unique[:10]
}
