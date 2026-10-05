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
	importConfirmationRule = "After authoring, review the result with the user and ask once. Name the target workspace (organization/name) and the authoring directory path, and list the changed files, each target address and import ID. State that one yes creates a speculative configuration version, uploads the reviewed archive and starts a plan-only run that cannot apply or change state, and covers create_import_cv, the upload, create_import_run and polling verify_import_plan. Ask again only if the archive or baseline changes."

	// importClosingRule is what the agent tells the user once the plan is verified.
	importClosingRule = "Tell the user the authoring directory path and that it holds the complete configuration, which may include sensitive files, so it should not be committed as is. The speculative run changed nothing: to make the import real, the user adds the blocks to the target workspace's normal source and workflow with their own review. The uploaded copy exists in HCP only as a speculative configuration version that these tools cannot download."
)
