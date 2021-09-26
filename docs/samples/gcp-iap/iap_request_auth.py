# -*- coding: utf-8 -*-
# Reference:
# https://cloud.google.com/iap/docs/authentication-howto#iap-make-request-python
# https://github.com/kubeflow/pipelines/blob/85cb99173dead8bd2ca09c8e040b137f59d00ad7/sdk/python/kfp/_auth.py#L183
# This is a helper function to make KFserving inferenceService
# call to a GCP cluster with IAP enabled.
# The id token is retrieved using `authenticate from desktop app` approach.

import argparse
import json
import logging
# from google.auth.transport.requests import Request
from webbrowser import open_new_tab

import requests

IAM_SCOPE = 'https://www.googleapis.com/auth/iam'
# OAUTH_TOKEN_URI = 'https://www.googleapis.com/oauth2/v4/token'
OAUTH_TOKEN_URI = 'https://oauth2.googleapis.com/token'

try:
    import http.client as http_client
except ImportError:
    # Python 2
    import httplib as http_client
http_client.HTTPConnection.debuglevel = 1

# You must initialize logging, otherwise you'll not see debug output.
logging.basicConfig()
logging.getLogger().setLevel(logging.DEBUG)
requests_log = logging.getLogger("requests.packages.urllib3")
requests_log.setLevel(logging.DEBUG)
requests_log.propagate = True


def getToken(iap_client_id, desktop_client_id, desktop_client_secret):
    token = None
    if desktop_client_id is None or desktop_client_secret is None:
        raise ValueError('desktop client id or secret is empty')
    else:
        # fetch IAP auth token: user account
        # Obtain the ID token for provided Client ID with user accounts.
        #  Flow: get authorization code ->
        #        exchange for refresh token ->
        #        obtain and return ID token
        refresh_token = getRefreshTokenFromClientId(desktop_client_id,
                                                    desktop_client_secret)

        token = idTokenFromRefreshToken(desktop_client_id,
                                        desktop_client_secret,
                                        refresh_token,
                                        iap_client_id)
    return token


def getRefreshTokenFromClientId(desktop_client_id, desktop_client_secret):
    auth_code = getAuthCode(desktop_client_id)
    auth_code = auth_code.strip()
    return getRefreshTokenFromCode(
            auth_code,
            desktop_client_id,
            desktop_client_secret)


def getAuthCode(client_id):
    auth_url = ('https://accounts.google.com/o/oauth2/v2/auth?' +
                'client_id=%s&response_type=code&scope=openid' +
                '%%20email&access_type=offline' +
                '&redirect_uri=urn:ietf:wg:oauth:2.0:oob') % client_id
    print(auth_url)
    open_new_tab(auth_url)
    return input("If there's no browser window prompt," +
                 " please direct to the URL above," +
                 "then copy and paste the authorization code here: ")


def getRefreshTokenFromCode(auth_code, client_id, client_secret):
    payload = {"code": auth_code,
               "client_id": client_id,
               "client_secret": client_secret,
               "redirect_uri": "urn:ietf:wg:oauth:2.0:oob",
               "grant_type": "authorization_code"}
    # req = requests.Request('POST',OAUTH_TOKEN_URI,data=payload)

    res = requests.post(OAUTH_TOKEN_URI, data=payload)
    print(res.text)
    return (str(json.loads(res.text)[u"refresh_token"]))


def idTokenFromRefreshToken(client_id, client_secret, refresh_token, audience):
    payload = {"client_id": client_id, "client_secret": client_secret,
               "refresh_token": refresh_token, "grant_type": "refresh_token",
               "audience": audience}
    res = requests.post(OAUTH_TOKEN_URI, data=payload)
    return (str(json.loads(res.text)[u"id_token"]))


def makeRequest(url, input_file, user_account, id_token):
    if input_file:
        with open(input_file) as f:
            data = f.read()
        resp = requests.post(
            url,
            verify=False,
            data=data,
            headers={
                'Authorization': 'Bearer {}'.format(id_token),
                'x-goog-authenticated-user-email': 'accounts.google.com:{}'
                .format(user_account)
            })
    else:
        resp = requests.get(
            url,
            verify=False,
            headers={'Authorization': 'Bearer {}'.format(id_token)})
    if resp.status_code == 403:
        raise Exception(
            'Service account {} does not have permission to '
            'access the IAP-protected application.'
            .format("var signer_email"))
    elif resp.status_code != 200:
        raise Exception('Bad response from application: {!r} / {!r} / {!r}'
                        .format(resp.status_code, resp.headers, resp.text))
    else:
        print(resp.text)


def main():
    # TODO: Use pythonfire to parse arguments.
    # fire.Fire()
    parser = argparse.ArgumentParser()
    parser.add_argument('--url', help='External URL of service endpoint')
    parser.add_argument('--iap_client_id',
                        help='The client id used to setup IAP')
    parser.add_argument('--desktop_client_id',
                        help='The client id for Desktop OAuth client')
    parser.add_argument('--desktop_client_secret',
                        help='The client secret for Desktop OAuth client')
    parser.add_argument('--user_account',
                        help='The user email address ' +
                        'which can access the namespace')
    parser.add_argument('--input',
                        help='The input file.')
    args = parser.parse_args()

    id_token = getToken(args.iap_client_id,
                        args.desktop_client_id,
                        args.desktop_client_secret)
    makeRequest(args.url, args.input, args.user_account, id_token)


if __name__ == "__main__":
    main()
