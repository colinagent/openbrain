package node

// func listSkillNodes(ctx context.Context) ([]*op.OpNode, error) {
// 	return listNodesByKind(ctx, nodestore.KindSkill)
// }

// func findSkillNodeByName(ctx context.Context, name string) (*op.OpNode, *op.SkillMeta, error) {
// 	name = strings.TrimSpace(name)
// 	if name == "" {
// 		return nil, nil, fmt.Errorf("skill name is required")
// 	}
// 	nodes, err := listSkillNodes(ctx)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	for _, node := range nodes {
// 		meta, ok := nodestore.NodeMeta[op.SkillMeta](node)
// 		if !ok {
// 			continue
// 		}
// 		if strings.EqualFold(strings.TrimSpace(meta.Name), name) {
// 			return node, meta, nil
// 		}
// 	}
// 	return nil, nil, fmt.Errorf("skill not found by name: %s", name)
// }
