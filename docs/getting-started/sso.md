# Single Sign-On(SSO)

> **Note**: This feature is only available in the **Team** plan.

**Single Sign-On (SSO)** is an authentication method that enables users to securely authenticate with multiple applications and websites by using just one set of credentials.

Slash supports SSO integration with **OAuth 2.0** standard.

## Create a new SSO provider

As an Admin user, you can create a new SSO provider in Setting > Workspace settings > SSO.

![sso-setting](../assets/getting-started/sso-setting.png)

For example, to integrate with GitHub, you might need to fill in the following fields:

![github-sso](../assets/getting-started/github-sso.png)

### Identity provider information

The information is the base concept of OAuth 2.0 and comes from your provider.

- **Client ID** is a public identifier of the custom provider;
- **Client Secret** is the OAuth2 client secret from identity provider;
- **Authorization endpoint** is the custom provider's OAuth2 login page address;
- **Token endpoint** is the API address for obtaining access token;
- **User endpoint** URL is the API address for obtaining user information by access token;
- **Scopes** is the scope parameter carried when accessing the OAuth2 URL, which is filled in according to the custom provider;

### User information mapping

For different providers, the structures returned by their user information API are usually not the same. In order to know how to map the user information from an provider into user fields, you need to fill the user information mapping form.

Slash will use the mapping to import the user profile fields when creating new accounts. The most important user field mapping is the identifier which is used to identify the Slash account associated with the OAuth 2.0 login.

- **Identifier** is the field name of primary email in 3rd-party user info;
- **Display name** is the field name of display name in 3rd-party user info (optional);

## Provider examples

### GitHub

| Field | Value |
|-------|-------|
| Authorization endpoint | `https://github.com/login/oauth/authorize` |
| Token endpoint | `https://github.com/login/oauth/access_token` |
| User endpoint | `https://api.github.com/user` |
| Scopes | `read:user user:email` |
| Identifier | `email` |
| Display name | `name` |

### Authentik

In Authentik, create an **OAuth2/OpenID Connect Provider** and a corresponding Application, then use the following values:

| Field | Value |
|-------|-------|
| Authorization endpoint | `https://<authentik-host>/application/o/<app-slug>/authorize/` |
| Token endpoint | `https://<authentik-host>/application/o/<app-slug>/token/` |
| User endpoint | `https://<authentik-host>/application/o/userinfo/` |
| Scopes | `openid email profile` |
| Identifier | `email` |
| Display name | `name` |

> The Client ID and Client Secret are found in your Authentik application's **OAuth2 Provider** settings.
> Authentik's userinfo endpoint returns `email`, `name`, and `preferred_username` among others — use `email` as the identifier since Slash requires a valid email address for each account.

### Google

| Field | Value |
|-------|-------|
| Authorization endpoint | `https://accounts.google.com/o/oauth2/v2/auth` |
| Token endpoint | `https://oauth2.googleapis.com/token` |
| User endpoint | `https://www.googleapis.com/oauth2/v3/userinfo` |
| Scopes | `openid email profile` |
| Identifier | `email` |
| Display name | `name` |
