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
//	                         the current configuration into, authors HCL in and
//	                         uploads from; its root is the archive root
//	scratch directory        a disposable agent-chosen directory for small checks;
//	                         never uploaded, never the archive root
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
	// importAuthoringDirectoryQuestion is how the agent asks for the authoring
	// directory when the target workspace has a current configuration.
	importAuthoringDirectoryQuestion = "Ask the user where the authoring directory should be: an existing empty directory, or a new directory you create at the path they give. You download the current configuration into it, author the HCL there and upload from it, so it persists. Never extract into a directory that already holds files and never overwrite the user's files; if it is not empty, ask for another path. Prefer a path outside any existing project or git repository and warn the user once if it is inside a git work tree. It will hold the complete configuration, which may include sensitive files. If the user already named the path in their instructions, use it after checking it is empty or new. If no user can answer and no path was supplied, stop; do not choose one yourself."

	// importBlankAuthoringDirectoryRule applies only when the server reports
	// that the target workspace has no current configuration and no state.
	importBlankAuthoringDirectoryRule = "The target workspace has no configuration yet, so nothing is downloaded and the authoring directory may already hold the user's Terraform files. Ask the user for it, and build an explicit file list to upload, not a copy of the whole directory: the .tf and .tf.json files, .terraform.lock.hcl and the modules the root module uses. Always exclude .env, .envrc, credential files, *.tfstate and backups, .terraform/ and .git/. Flag *.tfvars, *.auto.tfvars and other sensitive-looking files to the user and neither add nor drop them silently. Warn that resources already declared in the uploaded files will plan as creates, check that new addresses do not collide, and flag any backend or cloud block. Show the user the exact file list in the review."

	// importArchiveRootRule keeps the root structure of a downloaded
	// configuration so the target workspace's configuration root setting still
	// points at the right place.
	importArchiveRootRule = "Keep the archive root exactly as downloaded: extract with no path stripping and no wrapper folder, author the new blocks in the root module, and re-archive from the authoring directory root with the same relative paths. Exclude only generated .terraform caches. Never upload only the edited files, or an archive made from a subdirectory."

	// importConfigurationRootStop is the early stop for a target workspace whose
	// configuration root setting is not empty (SI-08).
	importConfigurationRootStop = "The target workspace has a configuration root setting (working_directory), which this workflow does not support yet because the root module is not the archive root. Stop and tell the user. Do not ask for an authoring directory, and do not create a configuration version or run."

	// importSensitiveFileRule covers sensitive files already in the download.
	importSensitiveFileRule = "If a sensitive file is already in the downloaded configuration, flag it to the user in the review and ask whether to keep or remove it. Never add a secret, copy a local-only secret into the upload, or remove or upload a sensitive file silently."

	// importScratchRule keeps disposable checks out of the authoring directory.
	importScratchRule = "Use a scratch directory, never the authoring directory, for small disposable checks such as terraform init -backend=false and terraform validate on a copy. Choose it yourself in the operating system's temporary area (ask the user for a path only if that is not permitted), never upload from it, and remove it when done. Never delete the authoring directory."

	// importValidationRule recommends local validation before upload.
	importValidationRule = "When a Terraform CLI is available, run terraform fmt and terraform validate in a scratch copy of the authoring directory before the upload, using terraform init -backend=false and the provider versions in .terraform.lock.hcl. It catches real problems before the speculative plan, so do it; if no CLI or provider is available, say so in the review. Do not run them after the upload."

	// importConfirmationRule is the single user review.
	importConfirmationRule = "After authoring, review the result with the user and ask once. Name the target workspace (organization/name) and the authoring directory path, and list the changed files, each adaptation you made to the suggested blocks and why, each target address and import ID. State that one yes creates a speculative configuration version, uploads the reviewed archive and starts a plan-only run that cannot apply or change state, and covers create_import_cv, the upload, create_import_run and polling verify_import_plan. Ask again only if the archive or baseline changes."

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
	importLockedProviderVersionRule = "The target workspace's provider versions are in the .terraform.lock.hcl of its downloaded configuration; Atlas does not record them. Read the exact version there, search_providers with that provider_version, then get_provider_details. If the archive has no lock file the version is uncertain: say so and fall back to the required_providers constraint. A blank workspace has no lock file yet, so use the version you choose and pin in the lock file you author."

	// importProviderMismatchRule compares the target's locked versions with the
	// Search source versions.
	importProviderMismatchRule = "Compare the provider versions in the .terraform.lock.hcl with the source versions in the carry block and tell the user about any mismatch. Never upgrade the target provider or Terraform to remove a mismatch."

	// importClosingRule is what the agent tells the user once the plan is verified.
	importClosingRule = "Tell the user the authoring directory path and that it holds the complete configuration, which may include sensitive files, so it should not be committed as is. The speculative run changed nothing: to make the import real, the user adds the blocks to the target workspace's normal source and workflow with their own review. The uploaded copy exists in HCP only as a speculative configuration version that these tools cannot download."
)

// retiredImportPhrases are wordings the shared vocabulary replaced (ADR 0009).
// They let one concept be named two ways, which confused agents and users.
var retiredImportPhrases = []string{
	"agent's workspace", "existing directory they approve", "approved empty directory",
	"which directory to use", "directory the user chose", "chosen directory", "chosen local directory",
	"destination workspace", "destination managed", "destination provider", "destination schema",
	"a local directory with the terraform cli", "optional and worthwhile",
}

// RetiredImportPhrases returns a copy of the retired wordings, lower-cased, so
// other packages can check their own tool text against the shared vocabulary.
func RetiredImportPhrases() []string {
	return append([]string(nil), retiredImportPhrases...)
}
