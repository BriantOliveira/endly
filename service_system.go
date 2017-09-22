package endly

import (
	"fmt"
	"github.com/viant/toolbox"
	"path"
	"strings"
)

const SystemServiceId = "system"
const (
	serviceTypeError = iota
	serviceTypeInitDaemon
	serviceTypeLaunchCtl
	serviceTypeStdService
	serviceTypeSystemctl
)

type ServiceStartRequest struct {
	Target    *Resource
	Service   string
	Exclusion string
}

type ServiceStopRequest struct {
	Target    *Resource
	Service   string
	Exclusion string
}

type ServiceStatusRequest struct {
	Target    *Resource
	Service   string
	Exclusion string
}

type ServiceInfo struct {
	Service string
	Path    string
	Pid     int
	Type    int
	Init    string
	State   string
}

func (s *ServiceInfo) IsActive() bool {
	return s.State == "running"
}

type systemService struct {
	*AbstractService
}

func (s *systemService) Run(context *Context, request interface{}) *ServiceResponse {
	var response = &ServiceResponse{Status: "ok"}

	var err error
	switch actualRequest := request.(type) {
	case *ServiceStartRequest:
		response.Response, err = s.startService(context, actualRequest)
		if err != nil {
			response.Error = fmt.Sprintf("Failed to start service: %v, %v", actualRequest.Service, err)
		}
	case *ServiceStopRequest:
		response.Response, err = s.stopService(context, actualRequest)
		if err != nil {
			response.Error = fmt.Sprintf("Failed to stop service: %v, %v", actualRequest.Service, err)
		}
	case *ServiceStatusRequest:
		response.Response, err = s.checkService(context, actualRequest)
		if err != nil {
			response.Error = fmt.Sprintf("Failed to check status service: %v, %v", actualRequest.Service, err)
		}
	}
	if response.Error != "" {
		response.Status = "err"
	}
	return response
}

func (s *systemService) NewRequest(action string) (interface{}, error) {
	switch action {
	case "status":
		return &ServiceStatusRequest{}, nil
	case "start":
		return &ServiceStartRequest{}, nil
	case "stop":
		return &ServiceStopRequest{}, nil

	}
	return s.AbstractService.NewRequest(action)
}

func (s *systemService) determineServiceType(context *Context, service, exclusion string, target *Resource) (int, string, error) {
	if exclusion != "" {
		exclusion = " | grep -v " + exclusion
	}
	commandResult, err := context.Execute(target, &ManagedCommand{
		Executions: []*Execution{
			{
				Command: fmt.Sprintf("ls /Library/LaunchDaemons/ | grep %v %v", service, exclusion),
			},
		},
	})
	if err != nil {
		return 0, "", err
	}
	if !CheckNoSuchFileOrDirectory(commandResult.Stdout()) {
		file := strings.TrimSpace(commandResult.Stdout())
		if len(file) > 0 {
			servicePath := path.Join("/Library/LaunchDaemons/", file)
			return serviceTypeLaunchCtl, servicePath, nil
		}
	}

	commandResult, err = context.ExecuteAsSuperUser(target, &ManagedCommand{
		Executions: []*Execution{
			{
				Command: "service " + service + " status",
			},
		},
	})
	if err != nil {
		return 0, "", err
	}
	if !CheckCommandNotFound(commandResult.Stdout()) {
		return serviceTypeStdService, service, nil
	}
	commandResult, err = context.ExecuteAsSuperUser(target, &ManagedCommand{
		Executions: []*Execution{
			{
				Command: "systemctl status " + service,
			},
		},
	})
	if err != nil {
		return 0, "", err
	}

	if !CheckCommandNotFound(commandResult.Stdout()) {
		return serviceTypeSystemctl, service, nil
	}

	return serviceTypeError, "", nil
}

func extractServiceInfo(state map[string]string, info *ServiceInfo) {
	if pid, ok := state["pid"]; ok {
		info.Pid = toolbox.AsInt(pid)
	}
	if state, ok := state["state"]; ok {
		if strings.Contains(state, "inactive") {
			state = "not running"
		} else if strings.Contains(state, "active") {
			state = "running"
		}
		info.State = state
	}
	if path, ok := state["path"]; ok {
		info.Path = path
	}
}

func (s *systemService) checkService(context *Context, request *ServiceStatusRequest) (*ServiceInfo, error) {

	if request.Service == "" {
		return nil, fmt.Errorf("Service was empty")
	}
	serviceType, serviceInit, err := s.determineServiceType(context, request.Service, request.Exclusion, request.Target)
	if err != nil {
		return nil, err
	}

	target, err := context.ExpandResource(request.Target)
	if err != nil {
		return nil, err
	}

	var result = &ServiceInfo{
		Service: request.Service,
		Type:    serviceType,
		Init:    serviceInit,
	}
	command := ""

	switch serviceType {
	case serviceTypeError:
		return nil, fmt.Errorf("Unknown daemon service type")
	case serviceTypeLaunchCtl:

		exclusion := request.Exclusion
		if exclusion != "" {
			exclusion = " | grep -v " + exclusion
		}

		commandResult, err := context.ExecuteAsSuperUser(target, &ManagedCommand{
			Executions: []*Execution{
				{
					Command: fmt.Sprintf("launchctl list | grep %v %v", request.Service, exclusion),
					Extraction: DataExtractions{
						{
							Key:     "pid",
							RegExpr: "(\\d+).+",
						},
					},
					Error: []string{"Unrecognized"},
				},
				{
					Command: "launchctl procinfo $pid",
					Extraction: DataExtractions{
						{
							Key:     "path",
							RegExpr: "program path[\\s|\\t]+=[\\s|\\t]+([^\\s]+)",
						},
						{
							Key:     "state",
							RegExpr: "[\\s|\\t]+state[\\s|\\t]+=[\\s|\\t]+([^s]+)",
						},
					},
					Error: []string{"Unrecognized"},
				}},
		})
		if err != nil {
			return nil, err
		}

		extractServiceInfo(commandResult.Extracted, result)
		return result, nil

	case serviceTypeSystemctl:
		command = fmt.Sprintf("systemctl status %v ", serviceInit)
	case serviceTypeStdService:
		command = fmt.Sprintf("service %v status", serviceInit)
	case serviceTypeInitDaemon:
		command = fmt.Sprintf("%v status", serviceInit)
	}

	commandResult, err := context.ExecuteAsSuperUser(target, &ManagedCommand{
		Executions: []*Execution{
			{
				Command: command,
				Extraction: DataExtractions{
					{
						Key:     "pid",
						RegExpr: "[^└]+└─(\\d+).+",
					},
					{
						Key:     "state",
						RegExpr: "[\\s|\\t]+Active:\\s+(\\S+)",
					},
					{
						Key:     "path",
						RegExpr: "[^└]+└─\\d+[\\s\\t].(.+)",
					},
				},
			},
		},
	})

	if err != nil {
		return nil, err
	}
	extractServiceInfo(commandResult.Extracted, result)
	return result, nil

}

func (s *systemService) stopService(context *Context, request *ServiceStopRequest) (*ServiceInfo, error) {
	serviceInfo, err := s.checkService(context, &ServiceStatusRequest{
		Target:    request.Target,
		Service:   request.Service,
		Exclusion: request.Exclusion,
	})
	if err != nil {
		return nil, err
	}
	if !serviceInfo.IsActive() {
		return serviceInfo, nil
	}
	target, err := context.ExpandResource(request.Target)
	if err != nil {
		return nil, err
	}
	command := ""
	switch serviceInfo.Type {
	case serviceTypeError:
		return nil, fmt.Errorf("Unknown daemon service type")
	case serviceTypeLaunchCtl:
		command = fmt.Sprintf("launchctl unload -F %v", serviceInfo.Init)
	case serviceTypeSystemctl:
		command = fmt.Sprintf("systemctl stop %v ", serviceInfo.Init)
	case serviceTypeStdService:
		command = fmt.Sprintf("service %v stop", serviceInfo.Init)
	case serviceTypeInitDaemon:
		command = fmt.Sprintf("%v stop", serviceInfo.Init)
	}

	commandResult, err := context.ExecuteAsSuperUser(target, &ManagedCommand{
		Executions: []*Execution{
			{
				Command: command,
			},
		},
	})
	if CheckCommandNotFound(commandResult.Stdout()) {
		return nil, fmt.Errorf("%v", commandResult.Stdout)
	}
	return s.checkService(context, &ServiceStatusRequest{
		Target:    request.Target,
		Service:   request.Service,
		Exclusion: request.Exclusion,
	})
}

func (s *systemService) startService(context *Context, request *ServiceStartRequest) (*ServiceInfo, error) {
	serviceInfo, err := s.checkService(context, &ServiceStatusRequest{
		Target:    request.Target,
		Service:   request.Service,
		Exclusion: request.Exclusion,
	})
	if err != nil {
		return nil, err
	}
	if serviceInfo.IsActive() {
		return serviceInfo, nil
	}
	target, err := context.ExpandResource(request.Target)
	if err != nil {
		return nil, err
	}
	command := ""
	switch serviceInfo.Type {
	case serviceTypeError:
		return nil, fmt.Errorf("Unknown daemon service type")
	case serviceTypeLaunchCtl:
		command = fmt.Sprintf("launchctl load -F %v", serviceInfo.Init)
	case serviceTypeSystemctl:
		command = fmt.Sprintf("systemctl start %v ", serviceInfo.Init)
	case serviceTypeStdService:
		command = fmt.Sprintf("service %v start", serviceInfo.Init)
	case serviceTypeInitDaemon:
		command = fmt.Sprintf("%v start", serviceInfo.Init)
	}

	commandResult, err := context.ExecuteAsSuperUser(target, &ManagedCommand{
		Executions: []*Execution{
			{
				Command: command,
			},
		},
	})
	if CheckCommandNotFound(commandResult.Stdout()) {
		return nil, fmt.Errorf("%v", commandResult.Stdout)
	}
	return s.checkService(context, &ServiceStatusRequest{
		Target:    request.Target,
		Service:   request.Service,
		Exclusion: request.Exclusion,
	})
}

func NewSystemService() Service {
	var result = &systemService{
		AbstractService: NewAbstractService(SystemServiceId),
	}
	result.AbstractService.Service = result
	return result
}
