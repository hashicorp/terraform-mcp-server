// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package importworkflow

// Shared vocabulary for the Search-to-import tools (ADR 0009). Every tool
// description, instruction and next_action uses these terms and, where the same
// rule applies, these exact sentences, so an agent never sees one concept under
// two names.
//
//	target workspace         the HCP Terraform workspace being imported into
//	authoring directory      the user-chosen local directory the agent downloads
//	                         the current configuration into, authors HCL in, runs
//	                         the local CLI checks in and uploads from; its root is
//	                         the archive root. The user may name any path,
//	                         including a temporary one for testing
//	archive root             the top of the configuration tree, kept exactly as
//	                         downloaded
//	root module              where Terraform runs; the archive root while the
//	                         configuration root setting is empty
//	configuration root setting  the HCP working-directory field (response field
//	                         working_directory), not a local path
//
// No tool takes a local path: the server may run remotely and never touches the
// client's filesystem.
const (
	// The authoring directory question has a common part and two variants,
	// chosen by whether the target workspace has a configuration to download.
	importAuthoringDirectoryCommon = "Ask the user where the authoring directory should be. You work in it for everything: download or author the HCL, run the local Terraform checks, and upload from it. Any path the user names is fine, including a temporary one for testing. If the user already named the path in their instructions, use it. If no user can answer and no path was supplied, stop; do not choose one yourself."

	// importAuthoringDirectoryExisting applies when the target workspace has a
	// current configuration, which is downloaded into the directory.
	importAuthoringDirectoryExisting = "The target workspace already has a configuration, which you download into the directory, so it must be an existing empty directory, or a new directory you create at the path they give. It will hold the complete configuration, which may include sensitive files. Never extract into a directory that already holds files and never overwrite the user's files; if it is not empty, ask for another path. If the user already named a path, check it is empty or new first. Any path is fine, including one inside a git repository. Warn the user once that the downloaded configuration may contain secrets or other sensitive information (such as .env or .tfvars files, credential files or inline secrets), so they can prevent an unwanted credential leak, for example by not committing, pushing or sharing the directory as is. After the download, check the files for sensitive information and flag anything you find to the user. When the plan is verified, tell the user the directory holds the complete configuration, which may include sensitive files, so it should not be committed as is."

	// importAuthoringDirectoryNew applies when the target workspace has no
	// configuration and no state, so nothing is downloaded.
	importAuthoringDirectoryNew = "The target workspace is new (no configuration and no state), so nothing is downloaded. Tell the user that a directory that already holds their Terraform files, such as an existing project or a repository they are starting, is fine, or that you can create a new empty one. When the plan is verified, tell the user the files are their own, ready to review and commit to their own source, but not to commit .terraform/ or any state files, and to check .tfvars files for secrets first."

	// importAuthoringDirectoryQuestion is the question for a target workspace
	// with an existing configuration.
	importAuthoringDirectoryQuestion = importAuthoringDirectoryCommon + " " + importAuthoringDirectoryExisting

	// importNewWorkspaceDirectoryQuestion is the question for a new workspace.
	importNewWorkspaceDirectoryQuestion = importAuthoringDirectoryCommon + " " + importAuthoringDirectoryNew

	// importBlankAuthoringDirectoryRule applies only when the server reports
	// that the target workspace has no current configuration and no state.
	importBlankAuthoringDirectoryRule = "For this new workspace, upload an explicit file list, not a copy of the whole directory: the .tf and .tf.json files, .terraform.lock.hcl and the modules the root module uses. Always exclude .env, .envrc, credential files, *.tfstate and backups, .terraform/ and .git/. Flag *.tfvars, *.auto.tfvars and other sensitive-looking files to the user and neither add nor drop them silently. Warn that resources already declared in the uploaded files will plan as creates, check that new addresses do not collide, and flag any backend or cloud block. Show the user the exact file list in the review, before uploading."

	// importNewWorkspaceUploadRule is checked again at the upload step, because
	// for a new workspace the upload is the one place a secret can leave the
	// user's machine (the configuration version is stored in HCP).
	importNewWorkspaceUploadRule = "For a new workspace, check the files once more before you upload: no key or credential files (such as *.pem, *.key or *.tfbackend), no symlinks pointing out of the directory, and no inline credentials in the .tf files (provider keys, secrets in variable defaults). Flag anything you find to the user, and upload only the file list the user confirmed in the review."

	// importArchiveRootRule keeps the root structure of a downloaded
	// configuration so the target workspace's configuration root setting still
	// points at the right place.
	importArchiveRootRule = "Keep the archive root exactly as downloaded: extract with no path stripping and no wrapper folder, author the new blocks in the root module, and re-archive from the authoring directory root with the same relative paths. Exclude only generated .terraform caches. Never upload only the edited files, or an archive made from a subdirectory."

	// importConfigurationRootStop is the early stop for a target workspace whose
	// configuration root setting is not empty (SI-08).
	importConfigurationRootStop = "The target workspace has a configuration root setting (working_directory), which this workflow does not support yet because the root module is not the archive root. Stop and tell the user. Do not ask for an authoring directory, and do not create a configuration version or run."

	// importSensitiveFileRule covers sensitive files already in the download.
	importSensitiveFileRule = "After downloading, check the configuration for sensitive information: credential or secret files (.env, .envrc, *.tfvars, *.pem, *.key, *.tfbackend) and inline secrets in .tf files (provider keys, secrets in variable defaults). Check without printing secret values. If you find any, flag it to the user in the review by file and kind, and ask whether to keep or remove it. Never add a secret, copy a local-only secret into the upload, or remove or upload a sensitive file silently."

	// importSourceVersionRule keeps a different provider version out of the
	// lock file that is uploaded. It applies only where a lock file exists.
	importSourceVersionRule = "On a target workspace that already has a configuration or state, read source-side provider details from the documentation and the query evidence. Do not run terraform init for a different provider version in the authoring directory, because that could change the lock file you upload. A new workspace with no configuration and no state has no lock file to protect, so this does not apply to it."

	// importValidationRule recommends local validation before upload.
	importValidationRule = "When a Terraform CLI is available, run terraform fmt before the upload, then terraform init -backend=false and terraform validate in the authoring directory. For an existing configuration: " + importLockFileCheck + " For a new workspace, the lock file init writes is the one you upload. Show any change to .terraform.lock.hcl in the review, never use -upgrade, and never upload .terraform/. It catches real problems before the speculative plan, so do it; if no CLI or provider is available, say so in the review. Do not run these after the upload."

	// importConfirmationRule is the single user review.
	importConfirmationRule = "After authoring, review the result with the user and ask once. Name the target workspace (organization/name) and the authoring directory path, and list the changed files, each adaptation you made to the suggested blocks and why, with its evidence (in particular any change from an identity import to an id import), each target address and import ID. State that one yes creates a speculative configuration version, uploads the reviewed archive and starts a plan-only run that cannot apply or change state, and covers create_import_cv, the upload, create_import_run and polling verify_import_plan. Ask again only if the archive or baseline changes."

	// importKeepGeneratedRule makes the generated blocks the default, so an
	// adaptation needs evidence. It is provider-neutral: scoping values differ
	// by provider (region, project, location, subscription, account).
	importKeepGeneratedRule = "Keep the generated resource and import blocks as returned: every attribute, the identity form of the import block, and any scoping values the block carries (for example a region, project, location, subscription or account, depending on the provider). Change something only when the target provider schema, the documentation for the target provider version, or the target Terraform version shows it is incompatible, or terraform validate rejects it. Do not replace an identity import with an id import unless identity_support for that type is none or unknown. An id import carries no scoping values, so the provider resolves them from its own configuration (default region, project or subscription) and may look in the wrong place. When identity_support is supported and schema_source is present, it describes the target workspace's last plan, so do not assume the target provider lacks identity because the search used a newer provider version."

	// importKeepGeneratedShort opens next_action so the key decision is first.
	importKeepGeneratedShort = "Key rule: keep the generated blocks as returned (every attribute, the identity form of the import block and its scoping values such as region, project or subscription). Change one only on evidence of incompatibility, and never replace an identity import with an id import while identity_support is supported."

	// importTargetScopeRule separates the target workspace's resource shape from
	// what the QueryRun found, which are easy to confuse.
	importTargetScopeRule = "Scope: types, schema_source and target describe the target workspace: the resource shape (managed_schema, identity_schema) read from the run that produced its current state. Author against this shape. candidates and carry.providers describe the QueryRun: the resources it found and the provider version the no-code QueryRun ran with, which can differ from the target's and is not a reason to change the generated blocks."

	// importTargetScopeShort opens next_action.
	importTargetScopeShort = "Scope: types and schema_source are the target workspace's resource shape; candidates and carry.providers come from the QueryRun, whose provider version can differ from the target's."

	// importAdaptationGuidance points the agent at evidence for adapting the
	// suggested blocks; it deliberately states no blanket rule, because accepted
	// argument placement (for example provider on an import block) differs
	// between Terraform versions.
	importAdaptationGuidance = "Adapt the suggested resource and import blocks to the target using evidence, not fixed rules. Use (1) the target provider schema JSON (managed_schema from prepare_import, or the schema you obtained) for which attributes exist, which are computed-only and which the target's provider version supports; (2) the documentation of the target workspace's locked provider version (search_providers with that provider_version, then get_provider_details); (3) the Terraform language documentation for the target's Terraform version, whose URL is versioned (for example developer.hashicorp.com/terraform/language/v1.16.x/block/import for the import block, using target.terraform_version_setting or target.terraform_version_last_run); and (4) the output of terraform validate with the target's Terraform version. Generated blocks are drafts written for the source provider version, so they may include attributes the target does not know, computed-only values, null placeholders, or arguments such as provider whose accepted placement differs between Terraform versions. When the schema, the documentation or a validate diagnostic rejects something, follow it, make the smallest change and do not guess. Never upgrade the target provider or Terraform to avoid an adaptation."

	// importArchiveURLRule keeps the signed download URL out of command lines.
	importArchiveURLRule = "Keep the signed download URL out of command arguments, logs and printed output. For example, send curl its configuration on stdin: curl --config - with the lines url = \"<download_url>\" and output = \"<archive file in the authoring directory>\" on stdin, so the URL stays out of the process list and shell history. This reduces exposure but does not remove it: the command text still passes through your tool harness and may be logged. If no safe method exists, stop and ask the user."

	// importSecretFilesRule keeps credentials in local files out of the output.
	importSecretFilesRule = "Do not echo, print or cat .env, .envrc, terraform.tfvars or credential files into the output. If one needs checking, confirm only that it exists."

	// importNothingCreated ends every response that did not create a
	// configuration version or run.
	importNothingCreated = "No CV or Run was created."

	// importNeverApplyRule bounds the guide-only and agent-schema local steps.
	importNeverApplyRule = "Never run apply or change workspace settings."

	// importLockFileCheck comes before any terraform init in a local directory,
	// because init would create a lock file and hide a missing one.
	importLockFileCheck = "Check the directory for .terraform.lock.hcl before terraform init: if it is absent, warn the user that terraform init will choose the newest allowed provider versions, which may differ from those that last changed this workspace, and continue only after explicit acknowledgement (label it lock_file_absent)."

	// importLockedProviderVersionRule says where the target's provider versions
	// come from and what to do when no lock file exists.
	importLockedProviderVersionRule = "The target workspace's provider versions are in the .terraform.lock.hcl of its downloaded configuration; Atlas does not record them. Read the exact version there, search_providers with that provider_version, then get_provider_details. If the archive has no lock file the version is uncertain: say so and fall back to the required_providers constraint. A blank workspace has no lock file yet, so use the version you choose and pin in the lock file you author. If schema_source.configuration_baseline_relation is not same_configuration_version, the returned shape may predate the current configuration: confirm against the documentation for the locked version before relying on identity_support."

	// importProviderMismatchRule compares the target's locked versions with the
	// Search source versions.
	importProviderMismatchRule = "Compare the provider versions in the .terraform.lock.hcl with the QueryRun's versions in the carry block and tell the user about any mismatch. A difference is expected and is not by itself a reason to change the generated blocks. Never upgrade the target provider or Terraform to remove a mismatch."

	// importDeleteDirectoryRule keeps authored work from being deleted.
	importDeleteDirectoryRule = "Do not delete or clean up the authoring directory without asking the user first."

	// importClosingRule is what the agent tells the user once the plan is verified.
	importClosingRule = "Tell the user the authoring directory path, and what to do with it as you were told when you asked for it. The speculative run changed nothing: to make the import real, the user adds the blocks to the target workspace's normal source and workflow with their own review. The uploaded copy exists in HCP only as a speculative configuration version that these tools cannot download."
)

// retiredImportPhrases are wordings the shared vocabulary replaced (ADR 0009).
// They let one concept be named two ways, which confused agents and users.
var retiredImportPhrases = []string{
	"agent's workspace", "existing directory they approve", "approved empty directory",
	"which directory to use", "directory the user chose", "chosen directory", "chosen local directory",
	"destination workspace", "destination managed", "destination provider", "destination schema",
	"a local directory with the terraform cli", "optional and worthwhile", "scratch directory", "scratch copy", "temporary area", "persistent location",
}

// RetiredImportPhrases returns a copy of the retired wordings, lower-cased, so
// other packages can check their own tool text against the shared vocabulary.
func RetiredImportPhrases() []string {
	return append([]string(nil), retiredImportPhrases...)
}
