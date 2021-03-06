package server

import (
	"../actions"
	"../proxy"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

type ServerTestSuite struct {
	BaseUrl        string
	ReconfigureUrl string // TODO: Remove
	ServiceName    string
	Server         *httptest.Server
	DnsIps         []string
	suite.Suite
}

func (s *ServerTestSuite) SetupTest() {
}

func TestServerUnitTestSuite(t *testing.T) {
	s := new(ServerTestSuite)
	s.ServiceName = "my-fancy-service"
	s.BaseUrl = "/v1/docker-flow-proxy/reconfigure"
	s.ReconfigureUrl = fmt.Sprintf(
		"%s?serviceName=%s&serviceColor=pink&servicePath=/path/to/my/service/api&serviceDomain=my-domain.com",
		s.BaseUrl,
		s.ServiceName,
	)
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actualPath := r.URL.Path
		if r.Method == "GET" {
			switch actualPath {
			case "/v1/docker-flow-proxy/reconfigure":
				if strings.EqualFold(r.URL.Query().Get("returnError"), "true") {
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					w.WriteHeader(http.StatusOK)
					w.Header().Set("Content-Type", "application/json")
				}
			case "/v1/docker-flow-proxy/remove":
				if strings.EqualFold(r.URL.Query().Get("returnError"), "true") {
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					w.WriteHeader(http.StatusOK)
					w.Header().Set("Content-Type", "application/json")
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))
	defer func() { s.Server.Close() }()

	lookupHostOrig := lookupHost
	defer func() { lookupHost = lookupHostOrig }()
	lookupHost = func(host string) (addrs []string, err error) {
		return s.DnsIps, nil
	}

	logPrintfOrig := logPrintf
	defer func() { logPrintf = logPrintfOrig }()
	logPrintf = func(format string, v ...interface{}) {}

	addr := strings.Replace(s.Server.URL, "http://", "", -1)
	s.DnsIps = []string{strings.Split(addr, ":")[0]}

	suite.Run(t, s)
}

// TestHandler

func (s *ServerTestSuite) Test_TestHandler_ReturnsStatus200() {
	for ver := 1; ver <= 2; ver++ {
		rw := getResponseWriterMock()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v%d/test", ver), nil)

		srv := Serve{}
		srv.TestHandler(rw, req)

		rw.AssertCalled(s.T(), "WriteHeader", 200)
	}
}

// ReconfigureHandler

func (s *ServerTestSuite) Test_ReconfigureHandler_SetsContentTypeToJSON() {
	var actual string
	httpWriterSetContentType = func(w http.ResponseWriter, value string) {
		actual = value
	}
	req, _ := http.NewRequest("GET", "/v1/docker-flow-proxy/reconfigure?serviceName=my-service", nil)

	srv := Serve{}
	srv.ReconfigureHandler(getResponseWriterMock(), req)

	s.Equal("application/json", actual)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_WritesErrorHeader_WhenReconfigureDistributeIsTrueAndError() {
	serve := Serve{}
	serve.Port = "1234"
	addr := "/v1/docker-flow-proxy/reconfigure?serviceName=my-service&distribute=true&servicePath=/demo"
	req, _ := http.NewRequest("GET", addr, nil)
	rw := getResponseWriterMock()
	sendDistributeRequestsOrig := SendDistributeRequests
	defer func() { SendDistributeRequests = sendDistributeRequestsOrig }()
	SendDistributeRequests = func(req *http.Request, port, serviceName string) (status int, err error) {
		return 0, fmt.Errorf("This is an error")
	}

	serve.ReconfigureHandler(rw, req)

	rw.AssertCalled(s.T(), "WriteHeader", 500)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_WritesStatusOK_WhenReconfigureDistributeIsTrue() {
	serve := Serve{}
	serve.Port = "1234"
	addr := "/v1/docker-flow-proxy/reconfigure?serviceName=my-service&distribute=true&servicePath=/demo"
	req, _ := http.NewRequest("GET", addr, nil)
	rw := getResponseWriterMock()
	sendDistributeRequestsOrig := SendDistributeRequests
	defer func() { SendDistributeRequests = sendDistributeRequestsOrig }()
	SendDistributeRequests = func(req *http.Request, port, serviceName string) (status int, err error) {
		return 0, nil
	}

	serve.ReconfigureHandler(rw, req)

	rw.AssertCalled(s.T(), "WriteHeader", 200)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_ReturnsStatus400_WhenServiceNameQueryIsNotPresent() {
	addr := "/v1/docker-flow-proxy/reconfigure"
	req, _ := http.NewRequest("GET", addr, nil)
	rw := getResponseWriterMock()

	srv := Serve{}
	srv.ReconfigureHandler(rw, req)

	rw.AssertCalled(s.T(), "WriteHeader", 400)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_ReturnsStatus200_WhenReqModeIsTcp() {
	addr := "/v1/docker-flow-proxy/reconfigure?serviceName=redis&port=6379&srcPort=6379&reqMode=tcp"
	req, _ := http.NewRequest("GET", addr, nil)
	rw := getResponseWriterMock()
	newReconfigureOrig := actions.NewReconfigure
	defer func() { actions.NewReconfigure = newReconfigureOrig }()
	actions.NewReconfigure = func(baseData actions.BaseReconfigure, serviceData proxy.Service, mode string) actions.Reconfigurable {
		return ReconfigureMock{
			ExecuteMock: func(args []string) error {
				return nil
			},
		}
	}

	srv := Serve{}
	srv.ReconfigureHandler(rw, req)

	rw.AssertCalled(s.T(), "WriteHeader", 200)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_ReturnsStatus400_WhenReqModeIsTcpAndSrcPortIsNotPresent() {
	addr := "/v1/docker-flow-proxy/reconfigure?serviceName=redis&port=6379&reqMode=tcp"
	req, _ := http.NewRequest("GET", addr, nil)
	rw := getResponseWriterMock()

	srv := Serve{}
	srv.ReconfigureHandler(rw, req)

	rw.AssertCalled(s.T(), "WriteHeader", 400)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_ReturnsStatus400_WhenReqModeIsTcpAndPortIsNotPresent() {
	addr := "/v1/docker-flow-proxy/reconfigure?serviceName=redis&srcPort=6379&reqMode=tcp"
	req, _ := http.NewRequest("GET", addr, nil)
	rw := getResponseWriterMock()

	srv := Serve{}
	srv.ReconfigureHandler(rw, req)

	rw.AssertCalled(s.T(), "WriteHeader", 400)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_ReturnsStatus400_WhenServicePathQueryIsNotPresent() {
	url := "/v1/docker-flow-proxy/reconfigure?serviceName=my-service"
	req, _ := http.NewRequest("GET", url, nil)
	rw := getResponseWriterMock()

	srv := Serve{}
	srv.ReconfigureHandler(rw, req)

	rw.AssertCalled(s.T(), "WriteHeader", 400)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_ReturnsStatus400_WhenModeIsServiceAndPortIsNotPresent() {
	url := "/v1/docker-flow-proxy/reconfigure?serviceName=my-service&serviceColor=orange&servicePath=/demo&serviceDomain=my-domain.com"
	req, _ := http.NewRequest("GET", url, nil)
	rw := getResponseWriterMock()

	srv := Serve{Mode: "service"}
	srv.ReconfigureHandler(rw, req)

	rw.AssertCalled(s.T(), "WriteHeader", 400)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_ReturnsStatus400_WhenModeIsSwarmAndPortIsNotPresent() {
	addr := "/v1/docker-flow-proxy/reconfigure?serviceName=my-service&serviceColor=orange&servicePath=/demo&serviceDomain=my-domain.com"
	req, _ := http.NewRequest("GET", addr, nil)
	rw := getResponseWriterMock()

	srv := Serve{Mode: "swARM"}
	srv.ReconfigureHandler(rw, req)

	rw.AssertCalled(s.T(), "WriteHeader", 400)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_InvokesReconfigureExecute() {
	addr := "/v1/docker-flow-proxy/reconfigure?serviceName=my-service&servicePath.1=/demo&port.1=1234"
	req, _ := http.NewRequest("GET", addr, nil)
	invoked := false
	newReconfigureOrig := actions.NewReconfigure
	defer func() { actions.NewReconfigure = newReconfigureOrig }()
	actions.NewReconfigure = func(baseData actions.BaseReconfigure, serviceData proxy.Service, mode string) actions.Reconfigurable {
		return ReconfigureMock{
			ExecuteMock: func(args []string) error {
				invoked = true
				return nil
			},
		}
	}

	srv := Serve{Mode: "swarm"}
	srv.ReconfigureHandler(getResponseWriterMock(), req)

	s.True(invoked)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_DoesNotInvokeReconfigureExecute_WhenDistributeIsTrue() {
	addr := "/v1/docker-flow-proxy/reconfigure?serviceName=my-service&servicePath=/demo&port=1234&distribute=true"
	req, _ := http.NewRequest("GET", addr, nil)
	invoked := false
	newReconfigureOrig := actions.NewReconfigure
	defer func() { actions.NewReconfigure = newReconfigureOrig }()
	actions.NewReconfigure = func(baseData actions.BaseReconfigure, serviceData proxy.Service, mode string) actions.Reconfigurable {
		return ReconfigureMock{
			ExecuteMock: func(args []string) error {
				invoked = true
				return nil
			},
		}
	}

	srv := Serve{Mode: "swarm"}
	srv.ReconfigureHandler(getResponseWriterMock(), req)

	s.False(invoked)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_ReturnsStatus500_WhenReconfigureExecuteFails() {
	newReconfigureOrig := actions.NewReconfigure
	defer func() { actions.NewReconfigure = newReconfigureOrig }()
	actions.NewReconfigure = func(baseData actions.BaseReconfigure, serviceData proxy.Service, mode string) actions.Reconfigurable {
		return ReconfigureMock{
			ExecuteMock: func(args []string) error {
				return fmt.Errorf("This is an error")
			},
		}
	}
	rw := getResponseWriterMock()
	addr := "/v1/docker-flow-proxy/reconfigure?serviceName=my-service&servicePath=/demo&port=1234"
	req, _ := http.NewRequest("GET", addr, nil)

	srv := Serve{}
	srv.ReconfigureHandler(rw, req)

	rw.AssertCalled(s.T(), "WriteHeader", 500)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_InvokesPutCert_WhenServiceCertIsPresent() {
	actualCertName := ""
	expectedCert := "my-cert with new line \\n"
	actualCert := ""
	cert := CertMock{
		PutCertMock: func(certName string, certContent []byte) (string, error) {
			actualCertName = certName
			actualCert = string(certContent[:])
			return "", nil
		},
	}
	addr := fmt.Sprintf("/v1/docker-flow-proxy/reconfigure?serviceName=my-service&servicePath=/demo&port=1234&serviceCert=%s", expectedCert)
	req, _ := http.NewRequest("GET", addr, nil)

	srv := Serve{
		Mode: "swarm",
		Cert: cert,
	}
	srv.ReconfigureHandler(getResponseWriterMock(), req)

	s.Equal("my-service", actualCertName)
	s.Equal(strings.Replace(expectedCert, "\\n", "\n", -1), actualCert)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_InvokesPutCertWithDomainName_WhenServiceCertIsPresent() {
	actualCertName := ""
	expectedCert := "my-cert with new line \\n"
	actualCert := ""
	cert := CertMock{
		PutCertMock: func(certName string, certContent []byte) (string, error) {
			actualCertName = certName
			actualCert = string(certContent[:])
			return "", nil
		},
	}
	addr := fmt.Sprintf("/v1/docker-flow-proxy/reconfigure?serviceName=my-service&servicePath=/demo&port=1234&serviceDomain=my-domain.com&serviceCert=%s", expectedCert)
	req, _ := http.NewRequest("GET", addr, nil)

	srv := Serve{
		Mode: "swarm",
		Cert: cert,
	}
	srv.ReconfigureHandler(getResponseWriterMock(), req)

	s.Equal("my-domain.com", actualCertName)
	s.Equal(strings.Replace(expectedCert, "\\n", "\n", -1), actualCert)
}

func (s *ServerTestSuite) Test_ReconfigureHandler_InvokesReconfigureExecute_WhenConsulTemplatePathIsPresent() {
	sd := proxy.ServiceDest{
		ServicePath: []string{},
	}
	pathFe := "/path/to/consul/fe/template"
	pathBe := "/path/to/consul/be/template"
	var actualBase actions.BaseReconfigure
	expectedBase := actions.BaseReconfigure{
		ConsulAddresses: []string{"http://my-consul.com"},
	}
	var actualService proxy.Service
	expectedService := proxy.Service{
		ServiceName:          "my-service",
		ConsulTemplateFePath: pathFe,
		ConsulTemplateBePath: pathBe,
		ReqMode:              "http",
		ServiceDest:          []proxy.ServiceDest{sd},
	}
	newReconfigureOrig := actions.NewReconfigure
	defer func() { actions.NewReconfigure = newReconfigureOrig }()
	invoked := false
	actions.NewReconfigure = func(baseData actions.BaseReconfigure, serviceData proxy.Service, mode string) actions.Reconfigurable {
		actualBase = baseData
		actualService = serviceData
		return ReconfigureMock{
			ExecuteMock: func(args []string) error {
				invoked = true
				return nil
			},
		}
	}
	serverImpl := Serve{
		ConsulAddresses: []string{"http://my-consul.com"},
	}
	addr := fmt.Sprintf(
		"/v1/docker-flow-proxy/reconfigure?serviceName=my-service&consulTemplateFePath=%s&consulTemplateBePath=%s",
		pathFe,
		pathBe,
	)
	req, _ := http.NewRequest("GET", addr, nil)

	serverImpl.ReconfigureHandler(getResponseWriterMock(), req)

	s.Equal(expectedBase, actualBase)
	s.Equal(expectedService, actualService)
	s.True(invoked)
}

// ReloadHandler

func (s *ServerTestSuite) Test_ReloadHandler_ReturnsStatus200() {
	reload = ReloadMock{
		ExecuteMock: func(recreate bool, listenerAddr string) error {
			return nil
		},
	}
	for ver := 1; ver <= 2; ver++ {
		rw := getResponseWriterMock()
		req, _ := http.NewRequest("GET", "/v1/docker-flow-proxy/reload", nil)

		srv := Serve{}
		srv.ReloadHandler(rw, req)

		rw.AssertCalled(s.T(), "WriteHeader", 200)
	}
}

func (s *ServerTestSuite) Test_ReloadHandler_InvokesReload() {
	invoked := false
	reloadOrig := reload
	defer func() { reload = reloadOrig }()
	reload = ReloadMock{
		ExecuteMock: func(recreate bool, listenerAddr string) error {
			invoked = true
			return nil
		},
	}
	req, _ := http.NewRequest("GET", "/v1/docker-flow-proxy/reload", nil)

	srv := Serve{}
	srv.ReloadHandler(getResponseWriterMock(), req)

	s.True(invoked)
}

func (s *ServerTestSuite) Test_ReloadHandler_InvokesReloadWithRecreateParam() {
	actualRecreate := false
	actualListenerAddr := ""
	reloadOrig := reload
	defer func() { reload = reloadOrig }()
	reload = ReloadMock{
		ExecuteMock: func(recreate bool, listenerAddr string) error {
			actualRecreate = recreate
			actualListenerAddr = listenerAddr
			return nil
		},
	}
	req, _ := http.NewRequest("GET", "/v1/docker-flow-proxy/reload?recreate=true", nil)

	srv := Serve{}
	srv.ListenerAddress = "my-listener"
	srv.ReloadHandler(getResponseWriterMock(), req)

	s.True(actualRecreate)
	s.Empty(actualListenerAddr)
}

func (s *ServerTestSuite) Test_ReloadHandler_InvokesReloadWithFromListenerParam() {
	actualListenerAddr := ""
	reloadOrig := reload
	defer func() { reload = reloadOrig }()
	reload = ReloadMock{
		ExecuteMock: func(recreate bool, listenerAddr string) error {
			actualListenerAddr = listenerAddr
			return nil
		},
	}
	req, _ := http.NewRequest("GET", "/v1/docker-flow-proxy/reload?fromListener=true", nil)

	srv := Serve{}
	srv.ListenerAddress = "my-listener"
	srv.ReloadHandler(getResponseWriterMock(), req)

	s.Equal(srv.ListenerAddress, actualListenerAddr)
}

func (s *ServerTestSuite) Test_ReloadHandler_SetsContentTypeToJSON() {
	var actual string
	httpWriterSetContentType = func(w http.ResponseWriter, value string) {
		actual = value
	}
	req, _ := http.NewRequest("GET", "/v1/docker-flow-proxy/reload", nil)
	reload = ReloadMock{
		ExecuteMock: func(recreate bool, listenerAddr string) error {
			return nil
		},
	}

	srv := Serve{}
	srv.ReloadHandler(getResponseWriterMock(), req)

	s.Equal("application/json", actual)
}

func (s *ServerTestSuite) Test_ReloadHandler_ReturnsJSON() {
	expected, _ := json.Marshal(Response{
		Status: "OK",
	})
	req, _ := http.NewRequest("GET", "/v1/docker-flow-proxy/reload", nil)
	reload = ReloadMock{
		ExecuteMock: func(recreate bool, listenerAddr string) error {
			return nil
		},
	}
	respWriterMock := getResponseWriterMock()

	srv := Serve{}
	srv.ReloadHandler(respWriterMock, req)

	respWriterMock.AssertCalled(s.T(), "Write", []byte(expected))
}

// Remove Handler

func (s *ServerTestSuite) Test_RemoveHandler_SetsContentTypeToJSON() {
	var actual string
	httpWriterSetContentType = func(w http.ResponseWriter, value string) {
		actual = value
	}
	req, _ := http.NewRequest("GET", "/v1/docker-flow-proxy/remove", nil)

	srv := Serve{}
	srv.RemoveHandler(getResponseWriterMock(), req)

	s.Equal("application/json", actual)
}

func (s *ServerTestSuite) Test_RemoveHandler_ReturnsJSON() {
	expected, _ := json.Marshal(Response{
		Status:      "OK",
		ServiceName: "my-service",
	})
	addr := "/v1/docker-flow-proxy/remove?serviceName=my-service"
	req, _ := http.NewRequest("GET", addr, nil)
	respWriterMock := getResponseWriterMock()

	srv := Serve{}
	srv.RemoveHandler(respWriterMock, req)

	respWriterMock.AssertCalled(s.T(), "Write", []byte(expected))
}

func (s *ServerTestSuite) Test_RemoveHandler_ReturnsStatus400_WhenServiceNameIsNotPresent() {
	req, _ := http.NewRequest("GET", "/v1/docker-flow-proxy/remove", nil)
	respWriterMock := getResponseWriterMock()

	srv := Serve{}
	srv.RemoveHandler(respWriterMock, req)

	respWriterMock.AssertCalled(s.T(), "WriteHeader", 400)
}

func (s *ServerTestSuite) Test_RemoveHandler_InvokesRemoveExecute() {
	mockObj := getRemoveMock("")
	aclName := "my-acl"
	var actual actions.Remove
	expected := actions.Remove{
		ServiceName:     s.ServiceName,
		TemplatesPath:   "",
		ConfigsPath:     "",
		ConsulAddresses: []string{"http://1.2.3.4:1234"},
		InstanceName:    "proxy-test-instance",
		AclName:         aclName,
	}
	actions.NewRemove = func(
		serviceName, aclName, configsPath, templatesPath string,
		consulAddresses []string,
		instanceName, mode string,
	) actions.Removable {
		actual = actions.Remove{
			ServiceName:     serviceName,
			AclName:         aclName,
			TemplatesPath:   templatesPath,
			ConfigsPath:     configsPath,
			ConsulAddresses: consulAddresses,
			InstanceName:    instanceName,
			Mode:            mode,
		}
		return mockObj
	}
	url := fmt.Sprintf("/v1/docker-flow-proxy/remove?serviceName=%s&aclName=%s", s.ServiceName, aclName)
	req, _ := http.NewRequest("GET", url, nil)

	srv := Serve{
		ConsulAddresses: expected.ConsulAddresses,
		ServiceName:     expected.InstanceName,
	}
	srv.RemoveHandler(getResponseWriterMock(), req)

	s.Equal(expected, actual)
	mockObj.AssertCalled(s.T(), "Execute", []string{})
}

func (s *ServerTestSuite) Test_RemoveHandler_WritesErrorHeader_WhenRemoveDistributeIsTrueAndItFails() {
	addr := "/v1/docker-flow-proxy/remove?serviceName=my-service&distribute=true"
	req, _ := http.NewRequest("GET", addr, nil)
	respWriterMock := getResponseWriterMock()

	serve := Serve{}
	serve.RemoveHandler(respWriterMock, req)

	respWriterMock.AssertCalled(s.T(), "WriteHeader", 500)
}

// GetServiceFromUrl

func (s *ServerTestSuite) Test_GetServiceFromUrl_ReturnsProxyService() {
	expected := proxy.Service{
		ServiceName:           "serviceName",
		AclName:               "aclName",
		ServiceColor:          "serviceColor",
		ServiceCert:           "serviceCert",
		OutboundHostname:      "outboundHostname",
		ConsulTemplateFePath:  "consulTemplateFePath",
		ConsulTemplateBePath:  "consulTemplateBePath",
		PathType:              "pathType",
		ReqPathSearch:         "reqPathSearch",
		ReqPathReplace:        "reqPathReplace",
		TemplateFePath:        "templateFePath",
		TemplateBePath:        "templateBePath",
		TimeoutServer:         "timeoutServer",
		TimeoutTunnel:         "timeoutTunnel",
		ReqMode:               "reqMode",
		HttpsOnly:             true,
		XForwardedProto:       true,
		RedirectWhenHttpProto: true,
		HttpsPort:             1234,
		ServiceDomain:         []string{"domain1", "domain2"},
		SkipCheck:             true,
		Distribute:            true,
		SslVerifyNone:         true,
		ServiceDomainMatchAll: true,
		ServiceDest:           []proxy.ServiceDest{{ServicePath: []string{}}},
	}
	addr := fmt.Sprintf(
		"%s?serviceName=%s&aclName=%s&serviceColor=%s&serviceCert=%s&outboundHostname=%s&consulTemplateFePath=%s&consulTemplateBePath=%s&pathType=%s&reqPathSearch=%s&reqPathReplace=%s&templateFePath=%s&templateBePath=%s&timeoutServer=%s&timeoutTunnel=%s&reqMode=%s&httpsOnly=%t&xForwardedProto=%t&redirectWhenHttpProto=%t&httpsPort=%d&serviceDomain=%s&skipCheck=%t&distribute=%t&sslVerifyNone=%t&serviceDomainMatchAll=%t",
		s.BaseUrl,
		expected.ServiceName,
		expected.AclName,
		expected.ServiceColor,
		expected.ServiceCert,
		expected.OutboundHostname,
		expected.ConsulTemplateFePath,
		expected.ConsulTemplateBePath,
		expected.PathType,
		expected.ReqPathSearch,
		expected.ReqPathReplace,
		expected.TemplateFePath,
		expected.TemplateBePath,
		expected.TimeoutServer,
		expected.TimeoutTunnel,
		expected.ReqMode,
		expected.HttpsOnly,
		expected.XForwardedProto,
		expected.RedirectWhenHttpProto,
		expected.HttpsPort,
		strings.Join(expected.ServiceDomain, ","),
		expected.SkipCheck,
		expected.Distribute,
		expected.SslVerifyNone,
		expected.ServiceDomainMatchAll,
	)
	req, _ := http.NewRequest("GET", addr, nil)
	srv := Serve{}

	actual := srv.GetServiceFromUrl(req)

	s.Equal(expected, *actual)
}

func (s *ServerTestSuite) Test_GetServiceFromUrl_DefaultsReqModeToHttp() {
	req, _ := http.NewRequest("GET", s.BaseUrl, nil)
	srv := Serve{}

	actual := srv.GetServiceFromUrl(req)

	s.Equal("http", actual.ReqMode)
}

func (s *ServerTestSuite) Test_GetServiceFromUrl_GetsUsersFromProxyExtractUsersFromString() {
	user1 := proxy.User{
		Username:      "user1",
		Password:      "pass1",
		PassEncrypted: false,
	}
	user2 := proxy.User{
		Username:      "user2",
		Password:      "pass2",
		PassEncrypted: true,
	}
	extractUsersFromStringOrig := extractUsersFromString
	defer func() { extractUsersFromString = extractUsersFromStringOrig }()
	actualServiceName := ""
	expectedServiceName := "my-service"
	actualUsers := ""
	expectedUsers := "user1,user2"
	actualUsersPassEncrypted := false
	expectedUsersPassEncrypted := true
	actualSkipEmptyPassword := true
	expectedSkipEmptyPassword := false
	extractUsersFromString = func(serviceName, usersString string, usersPassEncrypted, skipEmptyPassword bool) []*proxy.User {
		actualServiceName = serviceName
		actualUsers = usersString
		actualUsersPassEncrypted = usersPassEncrypted
		actualSkipEmptyPassword = skipEmptyPassword
		return []*proxy.User{&user1, &user2}
	}
	addr := fmt.Sprintf(
		"%s?serviceName=%s&users=%s&usersPassEncrypted=%t",
		s.BaseUrl,
		expectedServiceName,
		expectedUsers,
		expectedUsersPassEncrypted,
	)
	req, _ := http.NewRequest("GET", addr, nil)
	srv := Serve{}

	actual := srv.GetServiceFromUrl(req)

	s.Contains(actual.Users, user1)
	s.Contains(actual.Users, user2)
	s.Equal(expectedServiceName, actualServiceName)
	s.Equal(expectedUsers, actualUsers)
	s.Equal(expectedUsersPassEncrypted, actualUsersPassEncrypted)
	s.Equal(expectedSkipEmptyPassword, actualSkipEmptyPassword)
}

// mergeUsers

func (s *ServerTestSuite) Test_UsersMerge_AllCases() {
	usersBasePathOrig := usersBasePath
	defer func() { usersBasePath = usersBasePathOrig }()
	usersBasePath = "../test_configs/%s.txt"
	users := mergeUsers("someService", "user1:pass1,user2:pass2", "", false, "", false)
	s.Equal(users, []proxy.User{
		{PassEncrypted: false, Password: "pass1", Username: "user1"},
		{PassEncrypted: false, Password: "pass2", Username: "user2"},
	})
	users = mergeUsers("someService", "user1:pass1,user2", "", false, "", false)
	//user without password will not be included
	s.Equal(users, []proxy.User{
		{PassEncrypted: false, Password: "pass1", Username: "user1"},
	})
	users = mergeUsers("someService", "user1:passWoRd,user2", "users", false, "", false)
	//user2 password will come from users file
	s.Equal(users, []proxy.User{
		{PassEncrypted: false, Password: "passWoRd", Username: "user1"},
		{PassEncrypted: false, Password: "pass2", Username: "user2"},
	})

	users = mergeUsers("someService", "user1:passWoRd,user2", "users", true, "", false)
	//user2 password will come from users file, all encrypted
	s.Equal(users, []proxy.User{
		{PassEncrypted: true, Password: "passWoRd", Username: "user1"},
		{PassEncrypted: true, Password: "pass2", Username: "user2"},
	})

	users = mergeUsers("someService", "user1:passWoRd,user2", "users", false, "user1:pass1,user2:pass2", false)
	//user2 password will come from users file, but not from global one
	s.Equal(users, []proxy.User{
		{PassEncrypted: false, Password: "passWoRd", Username: "user1"},
		{PassEncrypted: false, Password: "pass2", Username: "user2"},
	})

	users = mergeUsers("someService", "user1:passWoRd,user2", "", false, "user1:pass1,user2:pass2", false)
	//user2 password will come from global file
	s.Equal(users, []proxy.User{
		{PassEncrypted: false, Password: "passWoRd", Username: "user1"},
		{PassEncrypted: false, Password: "pass2", Username: "user2"},
	})

	users = mergeUsers("someService", "user1:passWoRd,user2", "", false, "user1:pass1,user2:pass2", true)
	//user2 password will come from global file, globals encrypted only
	s.Equal(users, []proxy.User{
		{PassEncrypted: false, Password: "passWoRd", Username: "user1"},
		{PassEncrypted: true, Password: "pass2", Username: "user2"},
	})

	users = mergeUsers("someService", "user1:passWoRd,user2", "", true, "user1:pass1,user2:pass2", true)
	//user2 password will come from global file, all encrypted
	s.Equal(users, []proxy.User{
		{PassEncrypted: true, Password: "passWoRd", Username: "user1"},
		{PassEncrypted: true, Password: "pass2", Username: "user2"},
	})

	users = mergeUsers("someService", "user1,user2", "", false, "", false)
	//no users found dummy one generated
	s.Equal(len(users), 1)
	s.Equal(users[0].Username, "dummyUser")

	users = mergeUsers("someService", "", "users", false, "", false)
	//Users from file only
	s.Equal(users, []proxy.User{
		{PassEncrypted: false, Password: "pass1", Username: "user1"},
		{PassEncrypted: false, Password: "pass2", Username: "user2"},
	})

	users = mergeUsers("someService", "", "", false, "user1:pass1,user2:pass2", false)
	//No users when only globals present
	s.Equal(len(users), 0)

}

// GetServicesFromEnvVars

func (s *ServerTestSuite) Test_GetServicesFromEnvVars_ReturnsServices() {
	service := proxy.Service{
		AclName:               "my-AclName",
		ConsulTemplateBePath:  "my-ConsulTemplateBePath",
		ConsulTemplateFePath:  "my-ConsulTemplateFePath",
		Distribute:            true,
		HttpsOnly:             true,
		HttpsPort:             1234,
		OutboundHostname:      "my-OutboundHostname",
		PathType:              "my-PathType",
		RedirectWhenHttpProto: true,
		ReqMode:               "my-ReqMode",
		ReqPathReplace:        "my-ReqPathReplace",
		ReqPathSearch:         "my-ReqPathSearch",
		ServiceCert:           "my-ServiceCert",
		ServiceDomain:         []string{"my-domain-1.com", "my-domain-2.com"},
		ServiceDomainMatchAll: true,
		ServiceName:           "my-ServiceName",
		SkipCheck:             true,
		SslVerifyNone:         true,
		TemplateBePath:        "my-TemplateBePath",
		TemplateFePath:        "my-TemplateFePath",
		TimeoutServer:         "my-TimeoutServer",
		TimeoutTunnel:         "my-TimeoutTunnel",
		XForwardedProto:       true,
		ServiceDest: []proxy.ServiceDest{
			{Port: "1111", ServicePath: []string{"my-path-11", "my-path-12"}, SrcPort: 1112},
		},
	}
	os.Setenv("DFP_SERVICE_ACL_NAME", service.AclName)
	os.Setenv("DFP_SERVICE_CONSUL_TEMPLATE_FE_PATH", service.ConsulTemplateFePath)
	os.Setenv("DFP_SERVICE_CONSUL_TEMPLATE_BE_PATH", service.ConsulTemplateBePath)
	os.Setenv("DFP_SERVICE_DISTRIBUTE", strconv.FormatBool(service.Distribute))
	os.Setenv("DFP_SERVICE_HTTPS_ONLY", strconv.FormatBool(service.HttpsOnly))
	os.Setenv("DFP_SERVICE_HTTPS_PORT", strconv.Itoa(service.HttpsPort))
	os.Setenv("DFP_SERVICE_OUTBOUND_HOSTNAME", service.OutboundHostname)
	os.Setenv("DFP_SERVICE_PATH_TYPE", service.PathType)
	os.Setenv("DFP_SERVICE_REDIRECT_WHEN_HTTP_PROTO", strconv.FormatBool(service.RedirectWhenHttpProto))
	os.Setenv("DFP_SERVICE_REQ_MODE", service.ReqMode)
	os.Setenv("DFP_SERVICE_REQ_PATH_REPLACE", service.ReqPathReplace)
	os.Setenv("DFP_SERVICE_REQ_PATH_SEARCH", service.ReqPathSearch)
	os.Setenv("DFP_SERVICE_SERVICE_CERT", service.ServiceCert)
	os.Setenv("DFP_SERVICE_SERVICE_DOMAIN", strings.Join(service.ServiceDomain, ","))
	os.Setenv("DFP_SERVICE_SERVICE_DOMAIN_MATCH_ALL", strconv.FormatBool(service.ServiceDomainMatchAll))
	os.Setenv("DFP_SERVICE_SERVICE_NAME", service.ServiceName)
	os.Setenv("DFP_SERVICE_SKIP_CHECK", strconv.FormatBool(service.SkipCheck))
	os.Setenv("DFP_SERVICE_SSL_VERIFY_NONE", strconv.FormatBool(service.SslVerifyNone))
	os.Setenv("DFP_SERVICE_TEMPLATE_BE_PATH", service.TemplateBePath)
	os.Setenv("DFP_SERVICE_TEMPLATE_FE_PATH", service.TemplateFePath)
	os.Setenv("DFP_SERVICE_TIMEOUT_SERVER", service.TimeoutServer)
	os.Setenv("DFP_SERVICE_TIMEOUT_TUNNEL", service.TimeoutTunnel)
	os.Setenv("DFP_SERVICE_X_FORWARDED_PROTO", strconv.FormatBool(service.XForwardedProto))
	os.Setenv("DFP_SERVICE_PORT", service.ServiceDest[0].Port)
	os.Setenv("DFP_SERVICE_SERVICE_PATH", strings.Join(service.ServiceDest[0].ServicePath, ","))
	os.Setenv("DFP_SERVICE_SRC_PORT", strconv.Itoa(service.ServiceDest[0].SrcPort))

	defer func() {
		os.Unsetenv("DFP_SERVICE_ACL_NAME")
		os.Unsetenv("DFP_SERVICE_CONSUL_TEMPLATE_BE_PATH")
		os.Unsetenv("DFP_SERVICE_CONSUL_TEMPLATE_FE_PATH")
		os.Unsetenv("DFP_SERVICE_DISTRIBUTE")
		os.Unsetenv("DFP_SERVICE_HTTPS_ONLY")
		os.Unsetenv("DFP_SERVICE_HTTPS_PORT")
		os.Unsetenv("DFP_SERVICE_OUTBOUND_HOSTNAME")
		os.Unsetenv("DFP_SERVICE_PATH_TYPE")
		os.Unsetenv("DFP_SERVICE_PORT")
		os.Unsetenv("DFP_SERVICE_REDIRECT_WHEN_HTTP_PROTO")
		os.Unsetenv("DFP_SERVICE_REQ_MODE")
		os.Unsetenv("DFP_SERVICE_REQ_PATH_REPLACE")
		os.Unsetenv("DFP_SERVICE_REQ_PATH_SEARCH")
		os.Unsetenv("DFP_SERVICE_SERVICE_CERT")
		os.Unsetenv("DFP_SERVICE_SERVICE_DOMAIN")
		os.Unsetenv("DFP_SERVICE_SERVICE_DOMAIN_MATCH_ALL")
		os.Unsetenv("DFP_SERVICE_SERVICE_NAME")
		os.Unsetenv("DFP_SERVICE_SERVICE_PATH")
		os.Unsetenv("DFP_SERVICE_SKIP_CHECK")
		os.Unsetenv("DFP_SERVICE_SRC_PORT")
		os.Unsetenv("DFP_SERVICE_SSL_VERIFY_NONE")
		os.Unsetenv("DFP_SERVICE_TEMPLATE_BE_PATH")
		os.Unsetenv("DFP_SERVICE_TEMPLATE_FE_PATH")
		os.Unsetenv("DFP_SERVICE_TIMEOUT_SERVER")
		os.Unsetenv("DFP_SERVICE_TIMEOUT_TUNNEL")
		os.Unsetenv("DFP_SERVICE_X_FORWARDED_PROTO")
	}()
	srv := Serve{}
	actual := srv.GetServicesFromEnvVars()

	s.Len(*actual, 1)
	s.Contains(*actual, service)
}

func (s *ServerTestSuite) Test_GetServicesFromEnvVars_ReturnsServicesWithIndexedData() {
	service := proxy.Service{
		ServiceName: "my-ServiceName",
		ServiceDest: []proxy.ServiceDest{
			{Port: "1111", ServicePath: []string{"my-path-11", "my-path-12"}, SrcPort: 1112},
			{Port: "2221", ServicePath: []string{"my-path-21", "my-path-22"}, SrcPort: 2222},
		},
	}
	os.Setenv("DFP_SERVICE_SERVICE_NAME", service.ServiceName)
	os.Setenv("DFP_SERVICE_PORT_1", service.ServiceDest[0].Port)
	os.Setenv("DFP_SERVICE_SERVICE_PATH_1", strings.Join(service.ServiceDest[0].ServicePath, ","))
	os.Setenv("DFP_SERVICE_SRC_PORT_1", strconv.Itoa(service.ServiceDest[0].SrcPort))
	os.Setenv("DFP_SERVICE_PORT_2", service.ServiceDest[1].Port)
	os.Setenv("DFP_SERVICE_SERVICE_PATH_2", strings.Join(service.ServiceDest[1].ServicePath, ","))
	os.Setenv("DFP_SERVICE_SRC_PORT_2", strconv.Itoa(service.ServiceDest[1].SrcPort))

	defer func() {
		os.Unsetenv("DFP_SERVICE_SERVICE_NAME")
		os.Unsetenv("DFP_SERVICE_PORT_1")
		os.Unsetenv("DFP_SERVICE_SERVICE_PATH_1")
		os.Unsetenv("DFP_SERVICE_SRC_PORT_1")
		os.Unsetenv("DFP_SERVICE_PORT_2")
		os.Unsetenv("DFP_SERVICE_SERVICE_PATH_2")
		os.Unsetenv("DFP_SERVICE_SRC_PORT_2")
	}()
	srv := Serve{}
	actual := srv.GetServicesFromEnvVars()

	service.ReqMode = "http"
	s.Len(*actual, 1)
	s.Contains(*actual, service)
}

func (s *ServerTestSuite) Test_GetServicesFromEnvVars_ReturnsEmptyIfServiceNameIsNotSet() {
	srv := Serve{}
	actual := srv.GetServicesFromEnvVars()

	s.Len(*actual, 0)
}

func (s *ServerTestSuite) Test_GetServicesFromEnvVars_ReturnsMultipleServices() {
	service := proxy.Service{
		ServiceName: "my-ServiceName",
		ReqMode:     "http",
		ServiceDest: []proxy.ServiceDest{
			{Port: "1111", ServicePath: []string{"my-path-11", "my-path-12"}, SrcPort: 1112},
		},
	}
	os.Setenv("DFP_SERVICE_1_SERVICE_NAME", service.ServiceName)
	os.Setenv("DFP_SERVICE_1_PORT", service.ServiceDest[0].Port)
	os.Setenv("DFP_SERVICE_1_SERVICE_PATH", strings.Join(service.ServiceDest[0].ServicePath, ","))
	os.Setenv("DFP_SERVICE_1_SRC_PORT", strconv.Itoa(service.ServiceDest[0].SrcPort))
	defer func() {
		os.Unsetenv("DFP_SERVICE_1_SERVICE_NAME")
		os.Unsetenv("DFP_SERVICE_1_PORT")
		os.Unsetenv("DFP_SERVICE_1_SERVICE_PATH")
		os.Unsetenv("DFP_SERVICE_1_SRC_PORT")
	}()

	srv := Serve{}
	actual := srv.GetServicesFromEnvVars()

	s.Len(*actual, 1)
	s.Contains(*actual, service)
}

// Mocks

type ServerMock struct {
	mock.Mock
}

func (m *ServerMock) GetServiceFromUrl(sd []proxy.ServiceDest, req *http.Request) proxy.Service {
	params := m.Called(sd, req)
	return params.Get(0).(proxy.Service)
}

func (m *ServerMock) TestHandler(w http.ResponseWriter, req *http.Request) {
	m.Called(w, req)
}

func (m *ServerMock) ReloadHandler(w http.ResponseWriter, req *http.Request) {
	m.Called(w, req)
}

func (m *ServerMock) RemoveHandler(w http.ResponseWriter, req *http.Request) {
	m.Called(w, req)
}

func getServerMock(skipMethod string) *ServerMock {
	mockObj := new(ServerMock)
	if skipMethod != "ReloadHandler" {
		mockObj.On("ReloadHandler", mock.Anything, mock.Anything)
	}
	if skipMethod != "RemoveHandler" {
		mockObj.On("RemoveHandler", mock.Anything, mock.Anything)
	}
	if skipMethod != "TestHandler" {
		mockObj.On("TestHandler", mock.Anything, mock.Anything)
	}
	return mockObj
}

type ReloadMock struct {
	ExecuteMock func(recreate bool, listenerAddr string) error
}

func (m ReloadMock) Execute(recreate bool, listenerAddr string) error {
	return m.ExecuteMock(recreate, listenerAddr)
}

type RemoveMock struct {
	mock.Mock
}

func (m *RemoveMock) Execute(args []string) error {
	params := m.Called(args)
	return params.Error(0)
}

func getRemoveMock(skipMethod string) *RemoveMock {
	mockObj := new(RemoveMock)
	if skipMethod != "Execute" {
		mockObj.On("Execute", mock.Anything).Return(nil)
	}
	return mockObj
}

type ReconfigureMock struct {
	ExecuteMock                    func(args []string) error
	GetDataMock                    func() (actions.BaseReconfigure, proxy.Service)
	ReloadServicesFromListenerMock func(addresses []string, instanceName, mode, listenerAddress string) error
	GetTemplatesMock               func(sr *proxy.Service) (front, back string, err error)
	GetServicesFromEnvVarsMock     func() []proxy.Service
}

func (m ReconfigureMock) Execute(args []string) error {
	return m.ExecuteMock(args)
}

func (m ReconfigureMock) GetData() (actions.BaseReconfigure, proxy.Service) {
	return m.GetDataMock()
}

func (m ReconfigureMock) ReloadServicesFromListener(addresses []string, instanceName, mode, listenerAddress string) error {
	return m.ReloadServicesFromListenerMock(addresses, instanceName, mode, listenerAddress)
}

func (m ReconfigureMock) GetTemplates(sr *proxy.Service) (front, back string, err error) {
	return m.GetTemplatesMock(sr)
}

func (m ReconfigureMock) GetServicesFromEnvVars() []proxy.Service {
	return m.GetServicesFromEnvVarsMock()
}

type CertMock struct {
	PutMock     func(http.ResponseWriter, *http.Request) (string, error)
	PutCertMock func(certName string, certContent []byte) (string, error)
	GetAllMock  func(w http.ResponseWriter, req *http.Request) (CertResponse, error)
	GetInitMock func() error
}

func (m CertMock) Put(w http.ResponseWriter, req *http.Request) (string, error) {
	return m.PutMock(w, req)
}

func (m CertMock) PutCert(certName string, certContent []byte) (string, error) {
	return m.PutCertMock(certName, certContent)
}

func (m CertMock) GetAll(w http.ResponseWriter, req *http.Request) (CertResponse, error) {
	return m.GetAllMock(w, req)
}

func (m CertMock) Init() error {
	return m.GetInitMock()
}
