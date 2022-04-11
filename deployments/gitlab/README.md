The runner is used by GitLab to build and test the project. The runner image is versioned.

To update the runner follow these steps:

1. Login `docker login gitdc.ee.guardtime.com:5005 -u <username> -p <access_token>`
2. Build `docker build -t gitdc.ee.guardtime.com:5005/alphabill/alphabill/ab-gitlab-runner:$( date +"%Y%m%d") -f Dockerfile-runner ../..`
3. Upload `docker push gitdc.ee.guardtime.com:5005/alphabill/alphabill/ab-gitlab-runner:$( date +"%Y%m%d")`

Variables:

* username - GitLab username
* access_token - GitLab user specific access
  token. [Documentation](https://docs.gitlab.com/ee/user/profile/personal_access_tokens.html)
