package terraform

import (
	"log"

	"github.com/hashicorp/terraform/internal/addrs"
	"github.com/hashicorp/terraform/internal/checks"
	"github.com/hashicorp/terraform/internal/configs"
	"github.com/hashicorp/terraform/internal/dag"
	"github.com/hashicorp/terraform/internal/tfdiags"
)

type nodeExpandCheck struct {
	addr   addrs.ConfigCheck
	config *configs.Check

	reportChecks bool
	makeInstance func(addrs.AbsCheck, *configs.Check) dag.Vertex
}

var _ GraphNodeModulePath = (*nodeExpandCheck)(nil)
var _ GraphNodeDynamicExpandable = (*nodeExpandCheck)(nil)

func (n *nodeExpandCheck) ModulePath() addrs.Module {
	return n.addr.Module
}

func (n *nodeExpandCheck) DynamicExpand(ctx EvalContext) (*Graph, error) {
	exp := ctx.InstanceExpander()
	modInsts := exp.ExpandModule(n.ModulePath())

	instAddrs := addrs.MakeSet[addrs.Checkable]()
	var g Graph
	for _, modAddr := range modInsts {
		testAddr := n.addr.Check.Absolute(modAddr)
		log.Printf("[TRACE] nodeExpandCheck: Node for %s", testAddr)
		instAddrs.Add(testAddr)
		g.Add(n.makeInstance(testAddr, n.config))
	}
	addRootNodeToGraph(&g)

	if n.reportChecks {
		ctx.Checks().ReportCheckableObjects(n.addr, instAddrs)
	}

	return &g, nil
}

type nodeCheckAssert struct {
	addr   addrs.AbsCheck
	config *configs.Check
}

var _ GraphNodeModuleInstance = (*nodeCheckAssert)(nil)
var _ GraphNodeExecutable = (*nodeCheckAssert)(nil)

func (n *nodeCheckAssert) Path() addrs.ModuleInstance {
	return n.addr.Module
}

func (n *nodeCheckAssert) Execute(ctx EvalContext, _ walkOperation) tfdiags.Diagnostics {
	if status := ctx.Checks().ObjectCheckStatus(n.addr); status == checks.StatusFail || status == checks.StatusError {
		// This check is already failing, so we won't try and evaluate it.
		// This typically means there was an error in a data block within the
		// check block.
		return nil
	}

	// Otherwise, we'll go ahead and check the assert blocks.
	return evalCheckRules(
		addrs.CheckAssertion,
		n.config.Asserts,
		ctx,
		n.addr,
		EvalDataForNoInstanceKey,
		tfdiags.Warning)
}
