# Knative Serving Personas

When discussing user actions, it is often helpful to
[define specific user roles](<https://en.wikipedia.org/wiki/Persona_(user_experience)>)
who might want to do the action.

## Knative Serving Compute

### Developer Personas

The developer personas are software engineers looking to build and run a
stateless application without concern about the underlying infrastructure.
Developers expect to have tools which integrate with their native language
tooling or business processes.

- Hobbyist
- Backend SWE
- Full stack SWE
- SRE

User stories:

- Deploy some code
- Update environment
- Roll back the last change
- Debug an error in code
- Monitor my application

### Operator Personas

The operator personas are focused on deploying and managing both Knative Serving
and the underlying Kubernetes cluster, as well as applying organization policy
and security patches.

- Hobbyist / Contributor
- Cluster administrator
- Security Engineer / Auditor
- Capacity Planner

User stories:

- Create an Knative Serving cluster
- Apply policy / RBAC
- Control or charge back for resource usage
- Choose logging or monitoring plugins
- Audit or patch running Revisions

## Contributors

Contributors are an important part of the Knative Serving project. As such, we
will also consider how various infrastructure encourages and enables
contributors to the project, as well as the impact on end-users.

- Hobbyist or newcomer
- Motivated user
- Corporate (employed) maintainer
- Consultant

User stories:

- Check out the code
- Build and run the code
- Run tests
- View test status
- Run performance tests
