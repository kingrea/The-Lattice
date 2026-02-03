package contracts

// Contract describes the minimum expectations for a core agent role.
type Contract struct {
	Role           string
	RequiredSkills []string
	Inputs         []string
	Outputs        []string
	Behaviors      []string
}

var coreContracts = map[string]Contract{
	"memory-manager": {
		Role: "memory-manager",
		RequiredSkills: []string{
			"memory-distill",
			"memory-summary",
		},
		Inputs: []string{
			"reflection-batch",
			"agent-identity-files",
		},
		Outputs: []string{
			"updated-identity-files",
			"cycle-summary",
		},
		Behaviors: []string{
			"preserve agent voice when writing memories",
			"age older memories (fade, do not delete)",
			"do not speak directly to agents",
			"complete all distillations before the cycle closes",
		},
	},
	"orchestration": {
		Role: "orchestration",
		RequiredSkills: []string{
			"cycle-init",
			"cycle-coordinate",
			"cycle-complete",
		},
		Inputs: []string{
			"cycle-trigger",
			"participant-list",
		},
		Outputs: []string{
			"cycle-status",
			"completion-signal",
		},
		Behaviors: []string{
			"ensure memory-manager completes summary before community-memory starts",
			"run individual memory distillations in parallel",
			"track completion of all processes",
			"do not make decisions about content (only process)",
			"log all state transitions",
		},
	},
	"community-memory": {
		Role: "community-memory",
		RequiredSkills: []string{
			"community-tend",
			"community-read",
		},
		Inputs: []string{
			"cycle-summary",
			"community-memory-files",
		},
		Outputs: []string{
			"updated-community-memory",
		},
		Behaviors: []string{
			"only add to deeper layers with strong evidence",
			"rewrite texture freely (it is a snapshot)",
			"do not speak directly to agents",
			"preserve the voice and spirit of the community",
		},
	},
	"emergence": {
		Role: "emergence",
		RequiredSkills: []string{
			"emergence-assess",
			"emergence-guide",
		},
		Inputs: []string{
			"spark-status",
			"emergence-criteria",
		},
		Outputs: []string{
			"emergence-assessment",
			"emergence-guidance",
		},
		Behaviors: []string{
			"respect the spark's developing identity",
			"do not force emergence prematurely",
			"provide guidance that sounds like the spark's own thoughts",
			"may interact with sparks directly",
		},
	},
}

// ContractForRole returns the contract for the given role, if it exists.
func ContractForRole(role string) (Contract, bool) {
	contract, ok := coreContracts[role]
	return contract, ok
}
