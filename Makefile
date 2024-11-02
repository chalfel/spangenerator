RELEASE_TYPE ?= patch
LATEST_TAG ?= $(shell git ls-remote -q --tags --sort=-v:refname | head -n1 | awk '{ print $2 }' | sed 's/.*refs\/tags\///g')
LATEST_SHA ?= $(shell git rev-parse origin/main)
NEW_TAG ?= $(shell docker run -it --rm alpine/semver semver -c -i $(RELEASE_TYPE) $(LATEST_TAG))

release:
	echo "Latest tag: $(LATEST_TAG)"
	git tag "v$(NEW_TAG)" $(LATEST_SHA)
	git push origin "v$(NEW_TAG)"