.PHONY: publish

publish:
	@test -z "$$(git status --porcelain --untracked-files=no)" || { echo "Error: working tree has uncommitted changes"; exit 1; }
	@git push origin HEAD
	@git fetch --tags --quiet origin
	@last=$$(git tag --list 'v*' --sort=-v:refname | head -1); \
	  [ -n "$$last" ] || last=v0.0.0; \
	  ver=$${last#v}; \
	  major=$${ver%%.*}; \
	  rest=$${ver#*.}; \
	  minor=$${rest%%.*}; \
	  next="v$$major.$$((minor + 1)).0"; \
	  echo "Tagging $$next (previous: $$last)"; \
	  git tag "$$next" && git push origin "$$next"; \
	  echo "Pushed $$next; the release workflow will build and publish the binaries"
