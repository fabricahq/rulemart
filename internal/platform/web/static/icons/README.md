# Group icons (vendored)

The icons the pages show beside canonical groups. [catalog/group-icons.yaml](../../../../../catalog/group-icons.yaml)
says which group each icon belongs to. Only the icons it names are copied here, with each set's license or source.

| Directory | Source | Pinned at | License |
| --- | --- | --- | --- |
| `devicon/` | [Devicon](https://github.com/devicons/devicon), `icons/<name>/<file>.svg` | [v2.17.0](https://github.com/devicons/devicon/tree/v2.17.0), commit `54cfe13ac10eaa1ef817a343ab0a9437eb3c2e08` | MIT, in [devicon/LICENSE](devicon/LICENSE) |
| `community/` | A project's own logo where Devicon's doesn't read at a tile's size; see [community/NOTICE.md](community/NOTICE.md) | Per icon, in its notice | Per icon, in its notice |
| `lucide/` | [Lucide](https://github.com/lucide-icons/lucide), `icons/<file>.svg` | [1.49.0](https://github.com/lucide-icons/lucide/tree/1.49.0), commit `5a92b9ba262de5bf10e864219883267672c05db8` | ISC, in [lucide/LICENSE](lucide/LICENSE), which also holds the MIT license of the icons Lucide derives from Feather, such as `life-buoy` and `triangle-alert` |

Devicon's logos are the trademarks of their owners. Rulemart shows each only to identify the technology a group's
rules are about, which doesn't imply that the owner endorses Rulemart or the rules.

To add or change an icon, copy its file from the pinned version, or from a newer one after updating the pin for its
whole set here, and name it in `catalog/group-icons.yaml`. Never edit a Devicon or Lucide icon; a `community/` icon
changes only as its notice says. The pages show SVGs with `<img>`, which doesn't run scripts, and the tests reject an
SVG that holds scripts, event handlers, or references to other files, and any icon `catalog/group-icons.yaml` doesn't
name.
