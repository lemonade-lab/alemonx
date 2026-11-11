package mcp

import (
	"fmt"

	"alemonx/internal/workspace"
)

func WithRobotAccount(policy Policy, root string, resolveAccount func() (string, error)) Policy {
	policy.CheckAccount = func() error {
		_, err := resolveAccount()
		return err
	}
	policy.RobotList = func() (workspace.RobotList, error) {
		account, err := resolveAccount()
		if err != nil {
			return workspace.RobotList{}, err
		}
		return workspace.LoadRobots(root, account)
	}
	policy.RegisterRobot = func(path string) error {
		account, err := resolveAccount()
		if err != nil {
			return err
		}
		list, err := workspace.LoadRobots(root, account)
		if err != nil {
			return err
		}
		_, err = workspace.UpdateRobots(root, account, workspace.RobotListChange{
			Scope: list.Scope, Action: "add", Projects: []workspace.RobotEntry{{Path: path}},
		})
		if err != nil {
			return fmt.Errorf("机器人已创建，但无法保存到账户列表：%w", err)
		}
		return nil
	}
	return policy
}
