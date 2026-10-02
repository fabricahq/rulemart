Set up this project to follow the engineering rules I picked on Rulemart, using Code Rules (https://code-rules.fabricahq.com). Code Rules copies rules from libraries into `.code-rules/`, and generates `.code-rules/generated/RULES.md`, which says which rules to read before you work on this project.

Each library below is pinned with `ref` to the library release I saw on Rulemart, so its rules don't change until I upgrade them.

1. Run `code-rules --version`. It must print 0.2.0 or later. If Code Rules is missing or older, ask me before you install or upgrade it, as https://code-rules.fabricahq.com/start-here/install/ describes.
2. In the repository's root, run `code-rules project init`, unless `.code-rules/config.yaml` exists already.
3. Add these sources to `sources` in `.code-rules/config.yaml`. Keep every source and setting already there; a new project's file has `sources: {}`, which these replace.

   ```yaml
   sources:
     public-rules:
       repository: https://github.com/fabricahq/public-rules.git
       groups:
         - practices/testing
       rules:
         - techs/go/comment-non-obvious-struct-fields
         - techs/go/errors-include-useful-diagnostic-data
       ref: release/1
   ```

   If a source already imports one of these repositories, add the groups and rules to that source, under its name, instead of adding the repository again, and ask me before you change its `ref`. If another repository's source has one of these names, pick a name no source has.
4. Run `code-rules project sync`, then `code-rules project check`. If either fails, show me its error rather than working around it.
5. Check that the generated rules include what I picked, by these source-qualified rule IDs, with your source names if you changed them:
   - every rule of group `practices/testing` of `fabricahq/public-rules`, whose IDs start with `public-rules:practices/testing/`
   - `public-rules:techs/go/comment-non-obvious-struct-fields`
   - `public-rules:techs/go/errors-include-useful-diagnostic-data`
6. If `AGENTS.md`, `CLAUDE.md`, or the instruction file you read doesn't point to `.code-rules/generated/RULES.md` yet, add the section that `.code-rules/README.md` gives under "Connect your coding agent".
7. Tell me what you changed. Don't commit unless I ask; when I do, commit `.code-rules/` and the instruction file together.

To upgrade a library later, change its `ref` to a newer release's tag, such as `release/2`, and run `code-rules project sync`. To follow each rule's newest version instead, delete its `ref` line and run `code-rules project sync`; from then on, `code-rules project update` previews newer versions and applies them once I confirm.
