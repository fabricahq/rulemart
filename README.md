# Rulemart

Agent coding best practices, off the shelf.

Rulemart is a catalog of public [Code Rules](https://code-rules.fabricahq.com) libraries.
Browse rules other teams wrote for their agents, see who publishes and uses them, and add them to your project with one prompt.

## This branch

This branch holds an early prototype for exploring the user experience. It is not the product.

- [`prototype/`](prototype/) is a click-through mock of Rulemart. Every screen, including GitHub sign-in, app install, and issue creation, is simulated. All libraries, rules, and numbers are invented.
- [`docs/concept.html`](docs/concept.html) is the concept write-up: the mental model, UI walkthroughs, the feedback model, and the decisions made so far.

## Run the prototype

Open `prototype/index.html` in a browser, or serve the folder:

```sh
python3 -m http.server 8766 --directory prototype
```

Then visit <http://localhost:8766>. State is kept in your browser; use **Reset demo** in the footer to start over.
