package aad

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/versent/saml2aws/v2/pkg/cfg"
	"github.com/versent/saml2aws/v2/pkg/creds"
	"github.com/versent/saml2aws/v2/pkg/prompter"
	"github.com/versent/saml2aws/v2/pkg/provider"
)

var logger = logrus.WithField("provider", "AzureAD")

// Client wrapper around AzureAD enabling authentication and retrieval of assertions
type Client struct {
	provider.ValidateBase

	client     *provider.HTTPClient
	idpAccount *cfg.IDPAccount
}

// Autogenrated Converged Response struct
// for some cases, some fields may not exist
type ConvergedResponse struct {
	URLGetCredentialType    string             `json:"urlGetCredentialType"`
	ArrUserProofs           []userProof        `json:"arrUserProofs"`
	URLSkipMfaRegistration  string             `json:"urlSkipMfaRegistration"`
	OPerAuthPollingInterval map[string]float64 `json:"oPerAuthPollingInterval"`
	URLBeginAuth            string             `json:"urlBeginAuth"`
	URLEndAuth              string             `json:"urlEndAuth"`
	URLPost                 string             `json:"urlPost"`
	SErrorCode              string             `json:"sErrorCode"`
	SErrTxt                 string             `json:"sErrTxt"`
	SPOSTUsername           string             `json:"sPOST_Username"`
	SFT                     string             `json:"sFT"`
	SFTName                 string             `json:"sFTName"`
	SCtx                    string             `json:"sCtx"`
	Hpgact                  int                `json:"hpgact"`
	Hpgid                   int                `json:"hpgid"`
	Pgid                    string             `json:"pgid"`
	APICanary               string             `json:"apiCanary"`
	Canary                  string             `json:"canary"`
	CorrelationID           string             `json:"correlationId"`
	SessionID               string             `json:"sessionId"`
}

// Autogenerated GetCredentialType Request struct
// for some cases, some fields may not exist
type GetCredentialTypeRequest struct {
	Username                       string `json:"username"`
	IsOtherIdpSupported            bool   `json:"isOtherIdpSupported"`
	CheckPhones                    bool   `json:"checkPhones"`
	IsRemoteNGCSupported           bool   `json:"isRemoteNGCSupported"`
	IsCookieBannerShown            bool   `json:"isCookieBannerShown"`
	IsFidoSupported                bool   `json:"isFidoSupported"`
	OriginalRequest                string `json:"originalRequest"`
	Country                        string `json:"country"`
	Forceotclogin                  bool   `json:"forceotclogin"`
	IsExternalFederationDisallowed bool   `json:"isExternalFederationDisallowed"`
	IsRemoteConnectSupported       bool   `json:"isRemoteConnectSupported"`
	FederationFlags                int    `json:"federationFlags"`
	IsSignup                       bool   `json:"isSignup"`
	FlowToken                      string `json:"flowToken"`
	IsAccessPassSupported          bool   `json:"isAccessPassSupported"`
}

// Autogenerated GetCredentialType Response struct
// for some cases, some fields may not exist
type GetCredentialTypeResponse struct {
	Username       string `json:"Username"`
	Display        string `json:"Display"`
	IfExistsResult int    `json:"IfExistsResult"`
	IsUnmanaged    bool   `json:"IsUnmanaged"`
	ThrottleStatus int    `json:"ThrottleStatus"`
	Credentials    struct {
		PrefCredential        int         `json:"PrefCredential"`
		HasPassword           bool        `json:"HasPassword"`
		RemoteNgcParams       interface{} `json:"RemoteNgcParams"`
		FidoParams            interface{} `json:"FidoParams"`
		SasParams             interface{} `json:"SasParams"`
		CertAuthParams        interface{} `json:"CertAuthParams"`
		GoogleParams          interface{} `json:"GoogleParams"`
		FacebookParams        interface{} `json:"FacebookParams"`
		FederationRedirectURL string      `json:"FederationRedirectUrl"`
	} `json:"Credentials"`
	FlowToken          string `json:"FlowToken"`
	IsSignupDisallowed bool   `json:"IsSignupDisallowed"`
	APICanary          string `json:"apiCanary"`
}

// MFA Request struct
type mfaRequest struct {
	AuthMethodID       string `json:"AuthMethodId"`
	Method             string `json:"Method"`
	Ctx                string `json:"Ctx"`
	FlowToken          string `json:"FlowToken"`
	SessionID          string `json:"SessionId,omitempty"`
	AdditionalAuthData string `json:"AdditionalAuthData,omitempty"`
}

// MFA Response struct
type mfaResponse struct {
	Success       bool        `json:"Success"`
	ResultValue   string      `json:"ResultValue"`
	Message       interface{} `json:"Message"`
	AuthMethodID  string      `json:"AuthMethodId"`
	ErrCode       int         `json:"ErrCode"`
	Retry         bool        `json:"Retry"`
	FlowToken     string      `json:"FlowToken"`
	Ctx           string      `json:"Ctx"`
	SessionID     string      `json:"SessionId"`
	CorrelationID string      `json:"CorrelationId"`
	Timestamp     time.Time   `json:"Timestamp"`
	Entropy       int         `json:"Entropy"`
}

// A given method for a user to prove their indentity
type userProof struct {
	AuthMethodID string `json:"authMethodId"`
	Data         string `json:"data"`
	Display      string `json:"display"`
	IsDefault    bool   `json:"isDefault"`
}

// New create a new AzureAD client
func New(idpAccount *cfg.IDPAccount) (*Client, error) {

	tr := &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: idpAccount.SkipVerify, Renegotiation: tls.RenegotiateFreelyAsClient},
	}

	client, err := provider.NewHTTPClient(tr, provider.BuildHttpClientOpts(idpAccount))
	if err != nil {
		return nil, errors.Wrap(err, "error building http client")
	}

	return &Client{
		client:     client,
		idpAccount: idpAccount,
	}, nil
}

// Authenticate to AzureAD and return the data from the body of the SAML assertion.
func (ac *Client) Authenticate(loginDetails *creds.LoginDetails) (string, error) {
	var samlAssertion string
	var res *http.Response
	var err error
	var resBody []byte
	var resBodyStr string
	var convergedResponse *ConvergedResponse

	// idpAccount.URL = https://account.activedirectory.windowsazure.com

	// startSAML
	startURL := fmt.Sprintf("%s/applications/redirecttofederatedapplication.aspx?Operation=LinkedSignIn&applicationId=%s", ac.idpAccount.URL, ac.idpAccount.AppID)

	res, err = ac.client.Get(startURL)
	if err != nil {
		return samlAssertion, errors.Wrap(err, "error retrieving entry URL")
	}

AuthProcessor:
	for {
		resBody, _ = io.ReadAll(res.Body)
		resBodyStr = string(resBody)
		// reset res.Body so it can be read again later if required
		res.Body = io.NopCloser(bytes.NewBuffer(resBody))

		switch {
		case strings.Contains(resBodyStr, "ConvergedSignIn"):
			logger.Debug("processing ConvergedSignIn")
			res, err = ac.processConvergedSignIn(res, resBodyStr, loginDetails)
		case strings.Contains(resBodyStr, "ConvergedProofUpRedirect"):
			logger.Debug("processing ConvergedProofUpRedirect")
			res, err = ac.processConvergedProofUpRedirect(res, resBodyStr)
		case strings.Contains(resBodyStr, "KmsiInterrupt"):
			logger.Debug("processing KmsiInterrupt")
			res, err = ac.processKmsiInterrupt(res, resBodyStr)
		case strings.Contains(resBodyStr, "ConvergedTFA"):
			logger.Debug("processing ConvergedTFA")
			res, err = ac.processConvergedTFA(res, resBodyStr, loginDetails)
		case strings.Contains(resBodyStr, "SAMLRequest"):
			logger.Debug("processing SAMLRequest")
			res, err = ac.processSAMLRequest(res, resBodyStr)
		case ac.isHiddenForm(resBodyStr):
			if samlAssertion, _ = ac.getSamlAssertion(resBodyStr); samlAssertion != "" {
				logger.Debug("processing a SAMLResponse")
				return samlAssertion, nil
			}
			logger.Debug("processing a 'hiddenform'")
			res, err = ac.reProcessForm(resBodyStr)
		default:
			if strings.Contains(resBodyStr, "$Config") {
				if err := ac.unmarshalEmbeddedJson(resBodyStr, &convergedResponse); err != nil {
					return samlAssertion, errors.Wrap(err, "unmarshal error")
				}
				logger.Debug("unknown process step found:", convergedResponse.Pgid)
			} else {
				logger.Debug("reached an unknown page within the authentication process")
			}
			break AuthProcessor
		}
		if err != nil {
			return samlAssertion, err
		}
	}

	return samlAssertion, errors.New("failed get SAMLAssertion")
}

func (ac *Client) processConvergedSignIn(res *http.Response, srcBodyStr string, loginDetails *creds.LoginDetails) (*http.Response, error) {
	var convergedResponse *ConvergedResponse
	var err error

	if err := ac.unmarshalEmbeddedJson(srcBodyStr, &convergedResponse); err != nil {
		return res, errors.Wrap(err, "ConvergedSignIn response unmarshal error")
	}

	loginRequestUrl := ac.fullUrl(res, convergedResponse.URLPost)

	refererUrl := res.Request.URL.String()

	getCredentialTypeResponse, _, err := ac.requestGetCredentialType(refererUrl, loginDetails, convergedResponse)
	if err != nil {
		return res, errors.Wrap(err, "error processing GetCredentialType request")
	}

	federationRedirectURL := getCredentialTypeResponse.Credentials.FederationRedirectURL

	if federationRedirectURL != "" {
		res, err = ac.processADFSAuthentication(federationRedirectURL, loginDetails)
		if err != nil {
			return res, err
		}
	} else {
		res, err = ac.processAuthentication(loginRequestUrl, refererUrl, loginDetails, convergedResponse)
		if err != nil {
			return res, err
		}
	}

	return res, nil
}

func (ac *Client) requestGetCredentialType(refererUrl string, loginDetails *creds.LoginDetails, convergedResponse *ConvergedResponse) (GetCredentialTypeResponse, *http.Response, error) {
	var res *http.Response
	var getCredentialTypeResponse GetCredentialTypeResponse

	reqBodyObj := GetCredentialTypeRequest{
		Username:             loginDetails.Username,
		IsOtherIdpSupported:  true,
		CheckPhones:          false,
		IsRemoteNGCSupported: false,
		IsCookieBannerShown:  false,
		IsFidoSupported:      false,
		OriginalRequest:      convergedResponse.SCtx,
		FlowToken:            convergedResponse.SFT,
	}
	reqBodyJson, err := json.Marshal(reqBodyObj)
	if err != nil {
		return getCredentialTypeResponse, res, errors.Wrap(err, "failed to build GetCredentialType request JSON")
	}

	req, err := http.NewRequest("POST", convergedResponse.URLGetCredentialType, strings.NewReader(string(reqBodyJson)))
	if err != nil {
		return getCredentialTypeResponse, res, errors.Wrap(err, "error building GetCredentialType request")
	}

	req.Header.Add("canary", convergedResponse.APICanary)
	req.Header.Add("client-request-id", convergedResponse.CorrelationID)
	req.Header.Add("hpgact", fmt.Sprint(convergedResponse.Hpgact))
	req.Header.Add("hpgid", fmt.Sprint(convergedResponse.Hpgid))
	req.Header.Add("hpgrequestid", convergedResponse.SessionID)
	req.Header.Add("Referer", refererUrl)

	res, err = ac.client.Do(req)
	if err != nil {
		return getCredentialTypeResponse, res, errors.Wrap(err, "error retrieving GetCredentialType results")
	}

	err = json.NewDecoder(res.Body).Decode(&getCredentialTypeResponse)
	if err != nil {
		return getCredentialTypeResponse, res, errors.Wrap(err, "error decoding GetCredentialType results")
	}

	return getCredentialTypeResponse, res, nil
}

func (ac *Client) processADFSAuthentication(federationUrl string, loginDetails *creds.LoginDetails) (*http.Response, error) {
	var res *http.Response
	var err error
	var resBodyStr string
	var formValues url.Values
	var formSubmitUrl string
	var req *http.Request

	res, err = ac.client.Get(federationUrl)
	if err != nil {
		return res, errors.Wrap(err, "error retrieving ADFS url")
	}

	resBodyStr, _ = ac.responseBodyAsString(res.Body)

	formValues, formSubmitUrl, err = ac.reSubmitFormData(resBodyStr)
	if err != nil {
		return res, errors.Wrap(err, "failed to parse ADFS login form")
	}

	if formSubmitUrl == "" {
		return res, fmt.Errorf("unable to locate ADFS form submit URL")
	}

	formValues.Set("UserName", loginDetails.Username)
	formValues.Set("Password", loginDetails.Password)
	formValues.Set("AuthMethod", "FormsAuthentication")

	req, err = http.NewRequest("POST", ac.fullUrl(res, formSubmitUrl), strings.NewReader(formValues.Encode()))
	if err != nil {
		return res, errors.Wrap(err, "error building ADFS login request")
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err = ac.client.Do(req)
	if err != nil {
		return res, errors.Wrap(err, "error retrieving ADFS login results")
	}

	return res, nil
}

func (ac *Client) processAuthentication(loginUrl string, refererUrl string, loginDetails *creds.LoginDetails, convergedResponse *ConvergedResponse) (*http.Response, error) {
	var res *http.Response
	var err error
	var req *http.Request

	// 50058: user is not signed in (yet)
	if convergedResponse.SErrorCode != "" && convergedResponse.SErrorCode != "50058" {
		return res, fmt.Errorf("login error %s", convergedResponse.SErrorCode)
	}

	formValues := url.Values{}
	formValues.Set("canary", convergedResponse.Canary)
	formValues.Set("hpgrequestid", convergedResponse.SessionID)
	formValues.Set(convergedResponse.SFTName, convergedResponse.SFT)
	formValues.Set("ctx", convergedResponse.SCtx)
	formValues.Set("login", loginDetails.Username)
	formValues.Set("loginfmt", loginDetails.Username)
	formValues.Set("passwd", loginDetails.Password)

	req, err = http.NewRequest("POST", loginUrl, strings.NewReader(formValues.Encode()))
	if err != nil {
		return res, errors.Wrap(err, "error building login request")
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Referer", refererUrl)

	res, err = ac.client.Do(req)
	if err != nil {
		return res, errors.Wrap(err, "error retrieving login results")
	}

	return res, nil
}

func (ac *Client) processKmsiInterrupt(res *http.Response, srcBodyStr string) (*http.Response, error) {
	var convergedResponse *ConvergedResponse
	var err error

	if err := ac.unmarshalEmbeddedJson(srcBodyStr, &convergedResponse); err != nil {
		return res, errors.Wrap(err, "KMSI request unmarshal error")
	}

	formValues := url.Values{}
	formValues.Set(convergedResponse.SFTName, convergedResponse.SFT)
	formValues.Set("ctx", convergedResponse.SCtx)
	formValues.Set("LoginOptions", "1")

	req, err := http.NewRequest("POST", ac.fullUrl(res, convergedResponse.URLPost), strings.NewReader(formValues.Encode()))
	if err != nil {
		return res, errors.Wrap(err, "error building KMSI request")
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	ac.client.DisableFollowRedirect()
	res, err = ac.client.Do(req)
	if err != nil {
		return res, errors.Wrap(err, "error retrieving KMSI results")
	}
	ac.client.EnableFollowRedirect()

	return res, nil
}

func (ac *Client) processConvergedTFA(res *http.Response, srcBodyStr string, loginDetails *creds.LoginDetails) (*http.Response, error) {
	var convergedResponse *ConvergedResponse
	var err error

	if err := ac.unmarshalEmbeddedJson(srcBodyStr, &convergedResponse); err != nil {
		return res, errors.Wrap(err, "ConvergedTFA response unmarshal error")
	}

	mfas := convergedResponse.ArrUserProofs

	// if there's an explicit option to skip MFA, do so
	if convergedResponse.URLSkipMfaRegistration != "" {
		res, err = ac.client.Get(convergedResponse.URLSkipMfaRegistration)
		if err != nil {
			return res, errors.Wrap(err, "error retrieving skip MFA results")
		}
	} else if len(mfas) != 0 {
		// there's no explicit option to skip MFA, and MFA options are available
		res, err = ac.processMfa(mfas, convergedResponse, loginDetails)
		if err != nil {
			return res, err
		}
	}

	return res, nil
}

func (ac *Client) processMfa(mfas []userProof, convergedResponse *ConvergedResponse, loginDetails *creds.LoginDetails) (*http.Response, error) {
	var res *http.Response
	var err error
	var mfaResp mfaResponse

	if len(mfas) == 0 {
		return res, fmt.Errorf("MFA not found")
	}

	mfaResp, err = ac.processMfaBeginAuth(mfas, convergedResponse)
	if err != nil {
		return res, errors.Wrap(err, "error processing MFA BeginAuth")
	}

	for i := 0; ; i++ {
		mfaReq := mfaRequest{
			AuthMethodID: mfaResp.AuthMethodID,
			Method:       "EndAuth",
			Ctx:          mfaResp.Ctx,
			FlowToken:    mfaResp.FlowToken,
			SessionID:    mfaResp.SessionID,
		}
		if mfaReq.AuthMethodID == "PhoneAppOTP" || mfaReq.AuthMethodID == "OneWaySMS" {
			verifyCode := loginDetails.MFAToken
			if loginDetails.MFAToken == "" {
				verifyCode = prompter.StringRequired("Enter verification code")
			}
			mfaReq.AdditionalAuthData = verifyCode
		}
		if mfaReq.AuthMethodID == "PhoneAppNotification" && i == 0 {
			if mfaResp.Entropy == 0 {
				log.Println("Phone approval required.")
			} else {
				log.Printf("Phone approval required. Entropy is: %d", mfaResp.Entropy)
			}
		}

		mfaResp, err = ac.processMfaEndAuth(mfaReq, convergedResponse)
		if err != nil {
			return res, errors.Wrap(err, "error processing MFA EndAuth")
		}

		if mfaResp.ErrCode != 0 {
			return res, fmt.Errorf("error processing MFA, errcode: %d, message: %v", mfaResp.ErrCode, mfaResp.Message)
		}

		if mfaResp.Success {
			break
		}
		if !mfaResp.Retry {
			break
		}

		// if mfaResp.Retry == true then
		// must exist convergedResponse.OPerAuthPollingInterval[mfaResp.AuthMethodID]
		time.Sleep(time.Duration(convergedResponse.OPerAuthPollingInterval[mfaResp.AuthMethodID]) * time.Second)
	}

	if !mfaResp.Success {
		return res, fmt.Errorf("error processing MFA")
	}

	res, err = ac.processMfaAuth(mfaResp, convergedResponse)
	if err != nil {
		return res, errors.Wrap(err, "error processing MFA ProcessAuth")
	}

	return res, nil
}

func (ac *Client) processMfaBeginAuth(mfas []userProof, convergedResponse *ConvergedResponse) (mfaResponse, error) {
	var res *http.Response
	var err error
	var mfaResp mfaResponse
	var req *http.Request

	mfa := mfas[0]
	switch ac.idpAccount.MFA {
	case "Auto":
		for _, v := range mfas {
			if v.IsDefault {
				mfa = v
				break
			}
		}
	default:
		for _, v := range mfas {
			if v.AuthMethodID == ac.idpAccount.MFA {
				mfa = v
				break
			}
		}
	}
	mfaReqObj := mfaRequest{
		AuthMethodID: mfa.AuthMethodID,
		Method:       "BeginAuth",
		Ctx:          convergedResponse.SCtx,
		FlowToken:    convergedResponse.SFT,
	}
	mfaReqJson, err := json.Marshal(mfaReqObj)
	if err != nil {
		return mfaResp, errors.Wrap(err, "failed to build MFA BeginAuth request body")
	}

	req, err = http.NewRequest("POST", convergedResponse.URLBeginAuth, strings.NewReader(string(mfaReqJson)))
	if err != nil {
		return mfaResp, errors.Wrap(err, "error building MFA BeginAuth request")
	}

	req.Header.Add("Content-Type", "application/json")

	res, err = ac.client.Do(req)
	if err != nil {
		return mfaResp, errors.Wrap(err, "error retrieving MFA BeginAuth results")
	}

	err = json.NewDecoder(res.Body).Decode(&mfaResp)
	if err != nil {
		return mfaResp, errors.Wrap(err, "error decoding MFA BeginAuth results")
	}

	if !mfaResp.Success {
		return mfaResp, fmt.Errorf("MFA BeginAuth result is not success: %v", mfaResp.Message)
	}

	return mfaResp, nil
}

func (ac *Client) processMfaEndAuth(mfaReqObj mfaRequest, convergedResponse *ConvergedResponse) (mfaResponse, error) {
	var res *http.Response
	var err error
	var mfaResp mfaResponse
	var req *http.Request

	mfaReqJson, err := json.Marshal(mfaReqObj)
	if err != nil {
		return mfaResp, errors.Wrap(err, "failed to build MFA EndAuth request body")
	}

	req, err = http.NewRequest("POST", convergedResponse.URLEndAuth, strings.NewReader(string(mfaReqJson)))
	if err != nil {
		return mfaResp, errors.Wrap(err, "error building MFA EndAuth request")
	}

	req.Header.Add("Content-Type", "application/json")

	res, err = ac.client.Do(req)
	if err != nil {
		return mfaResp, errors.Wrap(err, "error retrieving MFA EndAuth results")
	}

	err = json.NewDecoder(res.Body).Decode(&mfaResp)
	if err != nil {
		return mfaResp, errors.Wrap(err, "error decoding MFA EndAuth results")
	}

	return mfaResp, nil
}

func (ac *Client) processMfaAuth(mfaResp mfaResponse, convergedResponse *ConvergedResponse) (*http.Response, error) {
	var res *http.Response
	var err error
	var req *http.Request

	formValues := url.Values{}
	formValues.Set(convergedResponse.SFTName, mfaResp.FlowToken)
	formValues.Set("request", mfaResp.Ctx)
	formValues.Set("login", convergedResponse.SPOSTUsername)

	req, err = http.NewRequest("POST", convergedResponse.URLPost, strings.NewReader(formValues.Encode()))
	if err != nil {
		return res, errors.Wrap(err, "error building MFA ProcessAuth request")
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err = ac.client.Do(req)
	if err != nil {
		return res, errors.Wrap(err, "error retrieving MFA ProcessAuth results")
	}

	return res, nil
}

func (ac *Client) processSAMLRequest(res *http.Response, srcBodyStr string) (*http.Response, error) {
	var err error

	// data is embedded javascript
	// window.location = 'https:/..../?SAMLRequest=......'
	oidcResponseList := strings.Split(srcBodyStr, ";")
	var samlRequestUrl string
	for _, v := range oidcResponseList {
		if strings.Contains(v, "SAMLRequest") {
			startURLPos := strings.Index(v, "https://")
			endURLPos := strings.Index(v[startURLPos:], "'")
			if endURLPos == -1 {
				endURLPos = strings.Index(v[startURLPos:], "\"")
			}
			samlRequestUrl = v[startURLPos : startURLPos+endURLPos]
		}
	}
	if samlRequestUrl == "" {
		return res, fmt.Errorf("unable to locate SAMLRequest URL")
	}

	res, err = ac.client.Get(samlRequestUrl)
	if err != nil {
		return res, errors.Wrap(err, "error retrieving SAMLRequest results")
	}

	return res, nil
}

func (ac *Client) processConvergedProofUpRedirect(res *http.Response, srcBodyStr string) (*http.Response, error) {
	var convergedResponse *ConvergedResponse
	var err error

	if err := ac.unmarshalEmbeddedJson(srcBodyStr, &convergedResponse); err != nil {
		return res, errors.Wrap(err, "skip MFA response unmarshal error")
	}

	// 50058: user is not signed in (yet)
	if convergedResponse.SErrorCode != "" && convergedResponse.SErrorCode != "50058" {
		return res, fmt.Errorf("login error %s", convergedResponse.SErrorCode)
	}

	if convergedResponse.URLSkipMfaRegistration == "" {
		return res, errors.Wrap(err, "skip MFA not possible")
	}

	res, err = ac.client.Get(convergedResponse.URLSkipMfaRegistration)
	if err != nil {
		return res, errors.Wrap(err, "error processing skip MFA request")
	}

	return res, nil
}

func (ac *Client) unmarshalEmbeddedJson(resBodyStr string, v any) error {
	/*
	 * data is embedded in a javascript object
	 * <script><![CDATA[  $Config=......; ]]>
	 */
	startIndex := strings.Index(resBodyStr, "$Config=") + 8
	return json.NewDecoder(strings.NewReader(resBodyStr[startIndex:])).Decode(&v)
}

func (ac *Client) responseBodyAsString(body io.ReadCloser) (string, error) {
	resBody, err := io.ReadAll(body)
	return string(resBody), err
}

func (ac *Client) fullUrl(res *http.Response, urlFragment string) string {
	if strings.HasPrefix(urlFragment, "/") {
		return res.Request.URL.Scheme + "://" + res.Request.URL.Host + urlFragment
	} else {
		return urlFragment
	}
}

func (ac *Client) isHiddenForm(resBodyStr string) bool {
	return strings.HasPrefix(resBodyStr, "<html><head><title>Working...</title>") && strings.Contains(resBodyStr, "name=\"hiddenform\"")
}

func (ac *Client) reProcessForm(srcBodyStr string) (*http.Response, error) {
	var res *http.Response
	var err error
	var formValues url.Values
	var formSubmitUrl string

	formValues, formSubmitUrl, err = ac.reSubmitFormData(srcBodyStr)
	if err != nil {
		return res, errors.Wrap(err, "failed to parse hiddenform form")
	}

	if formSubmitUrl == "" {
		return res, fmt.Errorf("unable to locate hiddenform submit URL")
	}

	req, err := http.NewRequest("POST", formSubmitUrl, strings.NewReader(formValues.Encode()))
	if err != nil {
		return res, errors.Wrap(err, "error building hiddenform request")
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err = ac.client.Do(req)
	if err != nil {
		return res, errors.Wrap(err, "error retrieving hiddenform results")
	}

	return res, nil
}

func (ac *Client) reSubmitFormData(resBodyStr string) (url.Values, string, error) {
	formValues := url.Values{}
	var formSubmitUrl string

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(resBodyStr))
	if err != nil {
		return formValues, formSubmitUrl, errors.Wrap(err, "failed to build document from response")
	}

	// prefil form data from page as provided
	doc.Find("input").Each(func(i int, s *goquery.Selection) {
		name, ok := s.Attr("name")
		if !ok {
			return
		}
		value, ok := s.Attr("value")
		if !ok {
			return
		}
		formValues.Set(name, value)
	})

	// identify form submit url/path
	doc.Find("form").Each(func(i int, s *goquery.Selection) {
		action, ok := s.Attr("action")
		if !ok {
			return
		}
		formSubmitUrl = action
	})

	return formValues, formSubmitUrl, nil
}

func (ac *Client) getSamlAssertion(resBodyStr string) (string, error) {
	var samlAssertion string

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(resBodyStr))
	if err != nil {
		return samlAssertion, errors.Wrap(err, "failed to build document from response")
	}

	doc.Find("input").Each(func(i int, s *goquery.Selection) {
		attrName, ok := s.Attr("name")
		if !ok {
			return
		}
		if attrName != "SAMLResponse" {
			return
		}
		samlAssertion, _ = s.Attr("value")
	})

	return samlAssertion, nil
}
