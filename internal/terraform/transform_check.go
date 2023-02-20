package terraform

import (
	"log"

	"github.com/hashicorp/terraform/internal/addrs"
	"github.com/hashicorp/terraform/internal/configs"
	"github.com/hashicorp/terraform/internal/dag"
)

type checkTransformer struct {
	// Config for the entire module.
	Config *configs.Config

	// We only report the checks during the plan, as the apply operation
	// remembers checks from the plan stage.
	ReportChecks bool
}

var _ GraphTransformer = (*checkTransformer)(nil)

func (t *checkTransformer) Transform(graph *Graph) error {
	return t.transform(graph, t.Config, graph.Vertices())
}

func (t *checkTransformer) transform(g *Graph, cfg *configs.Config, allNodes []dag.Vertex) error {
	moduleAddr := cfg.Path

	for _, check := range cfg.Module.Checks {
		configAddr := check.Addr().InModule(moduleAddr)
		node := &nodeExpandCheck{
			addr:         configAddr,
			config:       check,
			reportChecks: t.ReportChecks,
			makeInstance: func(addr addrs.AbsCheck, cfg *configs.Check) dag.Vertex {
				return &nodeCheckAssert{
					addr:   addr,
					config: cfg,
				}
			},
		}
		log.Printf("[TRACE] checkTransformer: Nodes and edges for %s", configAddr)
		g.Add(node)

		for _, other := range allNodes {
			if resource, isResource := other.(GraphNodeConfigResource); isResource {
				resourceAddr := resource.ResourceAddr()
				if !resourceAddr.Module.Equal(moduleAddr) {
					// This resource isn't in the same module as our check so
					// skip it.
					continue
				}

				resourceCfg := cfg.Module.ResourceByAddr(resourceAddr.Resource)
				if resourceCfg != nil && resourceCfg.Container != nil && resourceCfg.Container.Accessible(check.Addr()) {
					// Connect the check to the data source if the check can
					// access the data source.
					g.Connect(dag.BasicEdge(other, node))
					continue
				}
			}
		}
	}

	for _, child := range cfg.Children {
		if err := t.transform(g, child, allNodes); err != nil {
			return err
		}
	}

	return nil
}
