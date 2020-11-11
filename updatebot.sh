#!/bin/bash
jx step create pr go --name github.com/slimm609/go-scm --version ${VERSION} --build "make mod" --repo https://github.com/jenkins-x/lighthouse.git
