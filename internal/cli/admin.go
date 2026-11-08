package cli

import (
	"errors"

	"connectrpc.com/connect"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/spf13/cobra"
)

func (a *application) adminCommand() *cobra.Command {
	command := &cobra.Command{Use: "admin", Short: "Manage source-local accounts, groups, namespaces and taxonomy"}
	a.namespaceLifecycleCommands(command)
	user := a.remoteCommand("user-add USERNAME", "Create a source-local account", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
		stdin, _ := cmd.Flags().GetBool("password-stdin")
		if !stdin {
			return errors.New("use --password-stdin to supply the initial password")
		}
		password, err := passwordInput(cmd)
		if err != nil {
			return err
		}
		administrator, _ := cmd.Flags().GetBool("administrator")
		response, err := r.Admin.CreateUser(cmd.Context(), connect.NewRequest(&adminv1.CreateUserRequest{Mutation: a.mutation(), Username: args[0], Password: password, Administrator: administrator}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	})
	user.Flags().Bool("password-stdin", false, "read initial password from stdin")
	user.Flags().Bool("administrator", false, "grant Hub administrator access")
	command.AddCommand(user)
	for _, action := range []string{"user-disable", "user-enable"} {
		disabled := action == "user-disable"
		command.AddCommand(a.remoteCommand(action+" USER_ID", "Change account availability and revoke disabled sessions", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
			response, err := r.Admin.SetUserDisabled(cmd.Context(), connect.NewRequest(&adminv1.SetUserDisabledRequest{Mutation: a.mutation(), UserId: args[0], Disabled: disabled}))
			if err != nil {
				return err
			}
			return a.output(cmd, response.Msg)
		}))
	}
	command.AddCommand(a.remoteCommand("group-add NAME", "Create a permission group", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
		response, err := r.Admin.CreateGroup(cmd.Context(), connect.NewRequest(&adminv1.CreateGroupRequest{Mutation: a.mutation(), Name: args[0]}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	}))
	member := a.remoteCommand("group-member GROUP_ID USER_ID", "Add or remove a group member", cobra.ExactArgs(2), func(cmd *cobra.Command, r *client.Remote, args []string) error {
		remove, _ := cmd.Flags().GetBool("remove")
		response, err := r.Admin.SetGroupMember(cmd.Context(), connect.NewRequest(&adminv1.SetGroupMemberRequest{Mutation: a.mutation(), GroupId: args[0], UserId: args[1], Member: !remove}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	})
	member.Flags().Bool("remove", false, "revoke membership")
	command.AddCommand(member)
	namespace := a.remoteCommand("namespace-put NAME", "Create or update a namespace's review policy", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
		review, _ := cmd.Flags().GetBool("review-required")
		revision, _ := cmd.Flags().GetUint64("revision")
		response, err := r.Admin.PutNamespace(cmd.Context(), connect.NewRequest(&adminv1.PutNamespaceRequest{Mutation: a.mutation(), Name: args[0], ReviewRequired: review, ExpectedRevision: revision}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	})
	namespace.Flags().Bool("review-required", true, "require independent review")
	namespace.Flags().Uint64("revision", 0, "current revision, or zero to create")
	command.AddCommand(namespace)
	grant := a.remoteCommand("grant NAMESPACE SUBJECT_ID", "Grant or revoke one namespace role", cobra.ExactArgs(2), func(cmd *cobra.Command, r *client.Remote, args []string) error {
		kind, _ := cmd.Flags().GetString("kind")
		role, _ := cmd.Flags().GetString("role")
		remove, _ := cmd.Flags().GetBool("remove")
		subject := map[string]taxonomy.PrincipalKind{"user": taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, "group": taxonomy.PrincipalKind_PRINCIPAL_KIND_GROUP}[kind]
		permission := map[string]taxonomy.NamespaceRole{"reader": taxonomy.NamespaceRole_NAMESPACE_ROLE_READER, "maintainer": taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, "reviewer": taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, "admin": taxonomy.NamespaceRole_NAMESPACE_ROLE_ADMIN}[role]
		if subject == 0 || permission == 0 {
			return errors.New("invalid subject kind or namespace role")
		}
		response, err := r.Admin.SetNamespaceGrant(cmd.Context(), connect.NewRequest(&adminv1.SetNamespaceGrantRequest{Mutation: a.mutation(), Namespace: args[0], SubjectKind: subject, SubjectId: args[1], Role: permission, Granted: !remove}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	})
	grant.Flags().String("kind", "user", "user or group")
	grant.Flags().String("role", "", "reader, maintainer, reviewer, or admin")
	_ = grant.MarkFlagRequired("role")
	grant.Flags().Bool("remove", false, "revoke the role")
	command.AddCommand(grant)
	category := a.remoteCommand("category-put NAME", "Create, rename or archive a classification category", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		archived, _ := cmd.Flags().GetBool("archived")
		revision, _ := cmd.Flags().GetUint64("revision")
		response, err := r.Admin.PutCategory(cmd.Context(), connect.NewRequest(&adminv1.PutCategoryRequest{Mutation: a.mutation(), Name: args[0], Title: title, Archived: archived, ExpectedRevision: revision}))
		if err != nil {
			return err
		}
		return a.output(cmd, response.Msg)
	})
	category.Flags().String("title", "", "display title")
	_ = category.MarkFlagRequired("title")
	category.Flags().Bool("archived", false, "prevent new releases from using this category")
	category.Flags().Uint64("revision", 0, "current revision, or zero to create")
	command.AddCommand(category)
	a.addAccountCommands(command)
	a.addAdminLists(command)
	return command
}

func pageFlag(cmd *cobra.Command) *commonv1.PageRequest {
	token, _ := cmd.Flags().GetString("page-token")
	return &commonv1.PageRequest{PageSize: 100, PageToken: token}
}

func (a *application) addAdminLists(parent *cobra.Command) {
	type listing struct {
		use  string
		args cobra.PositionalArgs
		run  func(*cobra.Command, *client.Remote, []string) error
	}
	for _, item := range []listing{
		{"users", cobra.NoArgs, func(cmd *cobra.Command, r *client.Remote, args []string) error {
			response, err := r.Admin.ListUsers(cmd.Context(), connect.NewRequest(&adminv1.ListUsersRequest{Page: pageFlag(cmd)}))
			if err != nil {
				return err
			}
			return a.output(cmd, response.Msg)
		}},
		{"groups", cobra.NoArgs, func(cmd *cobra.Command, r *client.Remote, args []string) error {
			response, err := r.Admin.ListGroups(cmd.Context(), connect.NewRequest(&adminv1.ListGroupsRequest{Page: pageFlag(cmd)}))
			if err != nil {
				return err
			}
			return a.output(cmd, response.Msg)
		}},
		{"group-members GROUP_ID", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
			response, err := r.Admin.ListGroupMembers(cmd.Context(), connect.NewRequest(&adminv1.ListGroupMembersRequest{GroupId: args[0], Page: pageFlag(cmd)}))
			if err != nil {
				return err
			}
			return a.output(cmd, response.Msg)
		}},
		{"namespaces", cobra.NoArgs, func(cmd *cobra.Command, r *client.Remote, args []string) error {
			response, err := r.Admin.ListNamespaces(cmd.Context(), connect.NewRequest(&adminv1.ListNamespacesRequest{Page: pageFlag(cmd)}))
			if err != nil {
				return err
			}
			return a.output(cmd, response.Msg)
		}},
		{"grants NAMESPACE", cobra.ExactArgs(1), func(cmd *cobra.Command, r *client.Remote, args []string) error {
			response, err := r.Admin.ListNamespaceGrants(cmd.Context(), connect.NewRequest(&adminv1.ListNamespaceGrantsRequest{Namespace: args[0], Page: pageFlag(cmd)}))
			if err != nil {
				return err
			}
			return a.output(cmd, response.Msg)
		}},
		{"categories", cobra.NoArgs, func(cmd *cobra.Command, r *client.Remote, args []string) error {
			response, err := r.Admin.ListCategories(cmd.Context(), connect.NewRequest(&adminv1.ListCategoriesRequest{Page: pageFlag(cmd)}))
			if err != nil {
				return err
			}
			return a.output(cmd, response.Msg)
		}},
		{"audit", cobra.NoArgs, func(cmd *cobra.Command, r *client.Remote, args []string) error {
			response, err := r.Admin.ListAudit(cmd.Context(), connect.NewRequest(&adminv1.ListAuditRequest{Page: pageFlag(cmd)}))
			if err != nil {
				return err
			}
			return a.output(cmd, response.Msg)
		}},
	} {
		command := a.remoteCommand(item.use, "List source management records", item.args, item.run)
		command.Flags().String("page-token", "", "cursor returned by this source")
		parent.AddCommand(command)
	}
}
