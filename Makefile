%-release:
	@echo -e "Release $$(git semver --dryrun $*):\n" > /tmp/CHANGELOG
	@echo -e "$$(git log --pretty=format:"%h (%an): %s" $$(git describe --tags --abbrev=0 @^)..@)\n" >> /tmp/CHANGELOG
	@cat /tmp/CHANGELOG CHANGELOG > /tmp/NEW_CHANGELOG || :
	@mv /tmp/NEW_CHANGELOG CHANGELOG

	@sed -i 's#image: justinbarrick/matrixctl:.*#image: justinbarrick/matrixctl:$(shell git semver --dryrun $*)#g' deploy/kubernetes.yaml

	@git add CHANGELOG deploy/kubernetes.yaml
	@git commit -m "Release $(shell git semver --dryrun $*)"
	@git semver $*
