package cli

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/omushpapa/routerman/storage"
	"github.com/omushpapa/tplinkapi"
)

var (
	ErrInvalidChoice = errors.New("invalid choice")
	ErrInvalidInput  = errors.New("invalid input")
	ExitChoice       = 99
	QuitChoice       = 999
)

type Navigation int

const (
	NEXT Navigation = iota
	BACK
	REPEAT
)

type Context map[string]int

func (ctx Context) Set(key string, value int) {
	ctx[key] = value
}

type ActionFunc func(env *Env) (Navigation, error)

type Action struct {
	Name            string
	Children        []*Action
	RequiresContext []string
	Action          ActionFunc
}

func (action Action) GetValidChildren(ctx Context) []*Action {
	actions := make([]*Action, 0)

OUTER:
	for _, action := range action.Children {
		if len(action.RequiresContext) > 0 {
			for _, k := range action.RequiresContext {
				_, exists := ctx[k]
				if !exists {
					continue OUTER
				}
			}
		}
		actions = append(actions, action)
	}
	return actions
}

var RootActionManageUsers = &Action{
	Name: "Manage users",
	Children: []*Action{
		ActionRegisterUser,
		ActionListUsers,
	},
}

var ActionRegisterUser = &Action{
	Name: "Register a user",
	Action: func(env *Env) (Navigation, error) {
		fmt.Printf("Name: ")
		name, err := GetInput(env.in)
		if err != nil {
			return NEXT, err
		}
		if name == "" {
			return NEXT, ErrInvalidInput
		}
		user := &storage.User{
			Name: name,
		}
		err = env.db.UserStore.Create(user)
		if err != nil {
			return NEXT, err
		}
		fmt.Printf("user %+v created\n", user)
		return NEXT, nil
	},
}

var ActionListUsers = &Action{
	Name: "List users",
	Children: []*Action{
		ActionListUserBandwidthSlots,
		ActionDeregisterUser,
		ActionListDevices,
	},
	Action: func(env *Env) (Navigation, error) {
		var (
			pageNumber int  = 1
			pageSize   int  = 5
			showList   bool = true
			users      []storage.User
			err        error
		)

		for {
			if showList {
				users, err = env.db.UserStore.ReadMany(pageSize, pageNumber)
				if err != nil {
					return NEXT, err
				}

				if len(users) == 0 {
					if pageNumber == 1 {
						fmt.Println("no users found")
						return REPEAT, nil
					} else {
						fmt.Println("no more users found")
					}
				}

				dataRows := make([][]string, len(users))
				for i, user := range users {
					dataRows[i] = []string{user.Name}
				}
				err = PrintTable(env.out, dataRows, true, 2)
				if err != nil {
					return NEXT, err
				}
			} else {
				fmt.Println("no more users found")
			}

			fmt.Printf("\nSelect user by number or scroll with n(ext)/p(revious)/q(uit): ")
			choice, err := GetInput(env.in)
			if err != nil {
				return NEXT, err
			}

			switch choice {
			case "n":
				if len(users) == pageSize {
					pageNumber += 1
					showList = true
				} else {
					showList = false
				}
			case "p":
				if pageNumber > 1 {
					pageNumber -= 1
					showList = true
				} else {
					showList = false
				}
			case "q":
				return REPEAT, nil
			default:
				position, err := GetChoice(choice, len(users))
				if err == ErrInvalidChoice {
					fmt.Println("invalid choice. try again")
					showList = false
					continue
				}

				user := users[position]
				userId := user.Id

				fmt.Printf("Selected user '%s'\n", user.Name)

				_, err = env.db.UserStore.Read(userId)
				if err != nil {
					return NEXT, err
				}

				env.ctx.Set("userId", userId)
				return NEXT, err
			}
		}
	},
}

var ActionListUserBandwidthSlots = &Action{
	Name: "List user bandwidth slots",
	Children: []*Action{
		ActionRegisterDevice,
		ActionAssignSlot,
		ActionDeleteSlot,
	},
	RequiresContext: []string{"userId"},
	Action: func(env *Env) (Navigation, error) {
		userId, exists := env.ctx["userId"]
		if !exists {
			return NEXT, fmt.Errorf("user id not provided")
		}

		var (
			err        error
			slots      []storage.BandwidthSlot
			pageNumber int  = 1
			pageSize   int  = 5
			showList   bool = true
			choice     string
		)
		fmt.Println("\nListing user slots ")

		for {
			if showList {
				slots, err = env.db.BandwidthSlotStore.ReadManyByUserId(userId, pageSize, pageNumber)
				if err != nil {
					return NEXT, err
				}

				ids := make([]int, 0)
				for _, slot := range slots {
					ids = append(ids, slot.RemoteId)
				}

				entries, err := env.router.GetBwControlEntriesByList(ids)
				if err != nil {
					return NEXT, err
				}

				if len(slots) == 0 {
					fmt.Println("no slots found")
					return NEXT, nil
				}

				dataRows := make([][]string, len(entries))
				for i, entry := range entries {
					dataRows[i] = []string{
						fmt.Sprintf(
							"%s - %s Up:%d/%d Down:%d/%d [%v]\n",
							entry.StartIp, entry.EndIp, entry.UpMin, entry.UpMax, entry.DownMin, entry.DownMax, entry.Enabled,
						),
					}
				}
				err = PrintTable(env.out, dataRows, true, 2)
				if err != nil {
					return NEXT, err
				}
			} else {
				fmt.Println("no more slots found")
			}

			fmt.Printf("\nSelect slot by number or scroll with n(ext)/p(revious)/q(uit): ")
			choice, err = GetInput(env.in)
			if err != nil {
				return NEXT, err
			}

			switch choice {
			case "n":
				if len(slots) == pageSize {
					pageNumber += 1
					showList = true
				} else {
					showList = false
				}
			case "p":
				if pageNumber > 1 {
					pageNumber -= 1
					showList = true
				} else {
					showList = false
				}
			case "q":
				return REPEAT, nil
			default:
				position, err := GetChoice(choice, len(slots))
				if err == ErrInvalidChoice {
					fmt.Println("invalid choice. try again")
					showList = false
					continue
				}

				slotId := slots[position].Id
				_, err = env.db.BandwidthSlotStore.Read(slotId)
				if err != nil {
					return NEXT, err
				}

				env.ctx.Set("slotId", slotId)
				return NEXT, err
			}
		}
	},
}

var ActionAssignSlot = &Action{
	Name:            "Assign bandwidth slot",
	RequiresContext: []string{"userId"},
	Action: func(env *Env) (Navigation, error) {
		userId, exists := env.ctx["userId"]
		if !exists {
			return NEXT, fmt.Errorf("user id not provided")
		}

		var (
			err           error
			slots         []BwSlot
			pageNumber    int  = 1
			pageSize      int  = 5
			showList      bool = true
			choice        string
			useDhcpBounds bool
		)

	BOUNDS_LOOP:
		for {
			fmt.Printf("Use DHCP IP bounds (y/n): ")
			input, err := GetCharChoice(env.in, []string{"y", "n"})
			if err != nil {
				if err == ErrInvalidChoice {
					continue
				}
				return NEXT, err
			}
			switch input {
			case "y":
				useDhcpBounds = true
				break BOUNDS_LOOP
			case "n":
				useDhcpBounds = false
				break BOUNDS_LOOP
			default:
				fmt.Println("Invalid choice. Try again")
			}
		}

		for {
			if showList {
				slots, err = env.router.GetAvailableBandwidthSlots(useDhcpBounds)
				if err != nil {
					return NEXT, err
				}

				if len(slots) == 0 {
					fmt.Println("no slots found")
					nav := NEXT
					if pageNumber == 1 {
						nav = BACK
					}
					return nav, nil
				}

				for i, slot := range slots {
					cap, err := slot.GetCapacity()
					if err != nil {
						return NEXT, err
					}
					fmt.Printf("%d: %s - %s [%d]\n", i+1, slot.MinAddress, slot.MaxAddress, cap)
				}
			} else {
				fmt.Println("no more slots found")
			}

			fmt.Printf("\nSelect slot by number or scroll with n(ext)/p(revious)/q(uit): ")
			choice, err = GetInput(env.in)
			if err != nil {
				return NEXT, err
			}

			switch choice {
			case "n":
				if len(slots) == pageSize {
					pageNumber += 1
					showList = true
				} else {
					showList = false
				}
			case "p":
				if pageNumber > 1 {
					pageNumber -= 1
					showList = true
				} else {
					showList = false
				}
			case "q":
				return NEXT, nil
			default:
				position, err := GetChoice(choice, len(slots))
				if err == ErrInvalidChoice {
					return NEXT, fmt.Errorf("invalid choice")
				}
				slot := slots[position]

				fmt.Printf("Enter start IP [%s]: ", slot.MinAddress)
				startIPText, err := GetInput(env.in)
				if err != nil {
					return NEXT, err
				}
				var startIP string
				if startIPText == "" {
					startIP = slot.MinAddress
				} else {
					if !tplinkapi.IsValidIPv4Address(startIPText) {
						return REPEAT, fmt.Errorf("invalid IPv4 address")
					}
					startIPInt, err := tplinkapi.Ip2Int(startIPText)
					if err != nil {
						return NEXT, err
					}
					minIPInt, err := tplinkapi.Ip2Int(slot.MinAddress)
					if err != nil {
						return NEXT, err
					}
					if startIPInt < minIPInt {
						fmt.Printf("Given start IP is below range. Try again")
						return REPEAT, nil
					}
					maxIPInt, err := tplinkapi.Ip2Int(slot.MaxAddress)
					if err != nil {
						return NEXT, err
					}
					if startIPInt > maxIPInt {
						fmt.Printf("Given start IP is above range. Try again")
						return REPEAT, nil
					}
					startIP = startIPText
				}

				capacity, _ := slot.GetCapacity()
				fmt.Printf("Enter number of devices [Default %d]: ", capacity)
				num, err := GetIntInput(env.in, capacity)
				if err != nil {
					return NEXT, err
				}
				if num > capacity || num < 1 {
					return NEXT, fmt.Errorf("invalid number")
				}

				endIP, err := slot.GetMaxIP(startIP, num)
				if err != nil {
					return NEXT, err
				}

				maxDown := 1000
				fmt.Printf("Enter max download speed (kbps) [Default %d]: ", maxDown)
				maxDown, err = GetIntInput(env.in, maxDown)
				if err != nil {
					return NEXT, err
				}

				maxUp := 1000
				fmt.Printf("Enter max upload speed (kbps) [Default %d]: ", maxUp)
				maxUp, err = GetIntInput(env.in, maxUp)
				if err != nil {
					return NEXT, err
				}

				entry := tplinkapi.BandwidthControlEntry{
					Enabled: true,
					StartIp: startIP,
					EndIp:   endIP,
					UpMin:   50,
					UpMax:   maxUp,
					DownMin: 50,
					DownMax: maxDown,
				}
				id, err := env.router.service.AddBwControlEntry(entry)
				if err != nil {
					return NEXT, err
				}
				storageSlot := storage.BandwidthSlot{
					UserId:   userId,
					RemoteId: id,
				}
				err = env.db.BandwidthSlotStore.Create(&storageSlot)
				if err != nil {
					return NEXT, err
				}
				fmt.Println("Entry created successfully")
				return NEXT, err
			}
		}

	},
}

var ActionDeregisterUser = &Action{
	Name:            "Deregister user",
	RequiresContext: []string{"userId"},
	Action: func(env *Env) (Navigation, error) {
		userId, exists := env.ctx["userId"]
		if !exists {
			return NEXT, fmt.Errorf("user id not provided")
		}
		actions := []func(userId int) error{
			env.db.BandwidthSlotStore.DeleteByUserId,
			env.db.DeviceStore.DeleteByUserId,
			env.db.UserStore.Delete,
		}
		for _, action := range actions {
			err := action(userId)
			if err != nil {
				return NEXT, err
			}
		}
		fmt.Println("user deleted")
		delete(env.ctx, "userId")
		return BACK, nil
	},
}

var ActionDeleteSlot = &Action{
	Name: "Delete slot",
	Action: func(env *Env) (Navigation, error) {
		slotId, exists := env.ctx["slotId"]
		if !exists {
			return NEXT, fmt.Errorf("slot id not provided")
		}
		slot, err := env.db.BandwidthSlotStore.Read(slotId)
		if err != nil {
			return NEXT, err
		}
		err = env.router.service.DeleteBwControlEntry(slot.RemoteId)
		if err != nil {
			return NEXT, err
		}
		err = env.db.BandwidthSlotStore.Delete(slotId)
		if err != nil {
			return NEXT, err
		}
		fmt.Printf("slot deleted successfully")
		return BACK, nil
	},
	RequiresContext: []string{"slotId"},
}

var ActionListAvailableSlots = &Action{
	Name: "List available bandwidth slots",
	Action: func(env *Env) (Navigation, error) {
		slots, err := env.router.GetAvailableBandwidthSlots(true)
		if err != nil {
			return NEXT, err
		}
		dataRows := make([][]string, len(slots))
		for x, slot := range slots {
			cap, err := slot.GetCapacity()
			if err != nil {
				return NEXT, err
			}
			dataRows[x] = []string{
				fmt.Sprintf("%s - %s [%d]", slot.MinAddress, slot.MaxAddress, cap),
			}

		}
		err = PrintTable(env.out, dataRows, true, 2)
		return NEXT, err
	},
}

var RootActionManageDevices = &Action{
	Name: "Manage devices",
	Children: []*Action{
		ActionListDevices,
		ActionShowConnectedDevices,
		ActionExportARPBindings,
		ActionExportDhcpAddressReservations,
	},
}

var ActionListDevices = &Action{
	Name: "List devices",
	Children: []*Action{
		ActionDeregisterDevice,
	},
	Action: func(env *Env) (Navigation, error) {
		var (
			pageNumber int  = 1
			pageSize   int  = 5
			showList   bool = true
			devices    []storage.Device
			err        error
		)
		userId, userIdProvided := env.ctx["userId"]

		for {
			if showList {
				if userIdProvided && userId != 0 {
					devices, err = env.db.DeviceStore.ReadManyByUserId(userId, pageSize, pageNumber)
				} else {
					devices, err = env.db.DeviceStore.ReadMany(pageSize, pageNumber)
				}

				if err != nil {
					return NEXT, err
				}
				if len(devices) == 0 {
					fmt.Println("no devices found")
					return NEXT, nil
				}
				dataRows := make([][]string, len(devices))
				for i, device := range devices {
					user, err := device.GetUser(env.db.UserStore)
					var details string
					if err != nil {
						details = device.Alias
					} else {
						details = fmt.Sprintf("%s\t\t%s", device.Alias, user.Name)
					}
					dataRows[i] = []string{device.Mac, details}
				}
				err = PrintTable(env.out, dataRows, true, 3)
				if err != nil {
					return NEXT, err
				}
			} else {
				fmt.Println("no more users found")
			}

			fmt.Printf("\nSelect device by number or scroll with n(ext)/p(revious)/q(uit): ")
			choice, err := GetInput(env.in)
			if err != nil {
				return NEXT, err
			}
			switch choice {
			case "n":
				if len(devices) == pageSize {
					pageNumber += 1
					showList = true
				} else {
					showList = false
				}
			case "p":
				if pageNumber > 1 {
					pageNumber -= 1
					showList = true
				} else {
					showList = false
				}
			case "q":
				return NEXT, nil
			default:
				num, err := GetChoice(choice, len(devices))
				if err == ErrInvalidChoice {
					fmt.Println("invalid choice. try again")
					showList = false
					continue
				}

				deviceId := devices[num].Id
				env.ctx.Set("deviceId", deviceId)
				return NEXT, nil
			}
		}
	},
}

var ActionShowConnectedDevices = &Action{
	Name: "Show connected devices",
	Action: func(env *Env) (Navigation, error) {
		var (
			pageNumber int  = 1
			pageSize   int  = 5
			showList   bool = true
			stats      tplinkapi.ClientStatistics
			err        error
		)

		stats, err = env.router.service.GetStatistics()
		if err != nil {
			return NEXT, err
		}

		macAddresses := make([]string, len(stats))
		for _, stat := range stats {
			macAddresses = append(macAddresses, stat.Mac)
		}

		devices, err := env.db.DeviceStore.ReadManyByMac(macAddresses)
		if err != nil {
			return NEXT, err
		}

		deviceMap := make(map[string]storage.Device)
		for _, device := range devices {
			deviceMap[device.Mac] = device
		}

		for {
			if showList {
				if len(stats) == 0 {
					if pageNumber == 1 {
						fmt.Println("No connected devices")
						return NEXT, err
					} else {
						fmt.Println("No more devices found")
					}
				}

				dataRows := make([][]string, len(stats))
				for i, stat := range stats {
					device, exists := deviceMap[stat.Mac]
					details := "Unknown"
					if exists {
						user, err := device.GetUser(env.db.UserStore)
						if err != nil {
							details = device.Alias
						} else {
							details = fmt.Sprintf("%s\t\t%s", device.Alias, user.Name)
						}
					}
					dataRows[i] = []string{stat.IP, stat.Mac, details}
				}
				err = PrintTable(env.out, dataRows, true, 3)
				if err != nil {
					return NEXT, err
				}
			} else {
				fmt.Println("No more devices found")
			}

			fmt.Printf("\nScroll with n(ext)/p(revious)/q(uit): ")
			choice, err := GetInput(env.in)
			if err != nil {
				return NEXT, err
			}

			switch choice {
			case "n":
				if len(stats) == pageSize {
					pageNumber += 1
					showList = true
				} else {
					showList = false
				}
			case "p":
				if pageNumber > 1 {
					pageNumber -= 1
					showList = true
				} else {
					showList = false
				}
			case "q":
				return NEXT, nil
			default:
				fmt.Println("Invalid input")
				continue
			}
		}
	},
}

var ActionExportARPBindings = &Action{
	Name: "Export ARP Bindings",
	Action: func(env *Env) (Navigation, error) {
		var (
			bindings []tplinkapi.ClientReservation
			err      error
		)

		bindings, err = env.router.service.GetIpMacBindings()
		if err != nil {
			return NEXT, err
		}

		if len(bindings) == 0 {
			fmt.Println("No bindings found")
			return NEXT, nil
		}

		err = ExportBindings(bindings, "bindings.csv")
		return NEXT, err
	},
}

var ActionExportDhcpAddressReservations = &Action{
	Name: "Export DHCP Address Reservations",
	Action: func(env *Env) (Navigation, error) {
		var (
			reservations []tplinkapi.ClientReservation
			err          error
		)

		reservations, err = env.router.service.GetAddressReservations()
		if err != nil {
			return NEXT, err
		}

		if len(reservations) == 0 {
			fmt.Println("No reservations found")
			return NEXT, nil
		}

		err = ExportBindings(reservations, "reservations.csv")
		return NEXT, err
	},
}

var ActionRegisterDevice = &Action{
	Name:            "Register a device",
	RequiresContext: []string{"userId", "slotId"},
	Action: func(env *Env) (Navigation, error) {
		userId, exists := env.ctx["userId"]
		if !exists {
			return NEXT, fmt.Errorf("user id not provided")
		}

		slotId, exists := env.ctx["slotId"]
		if !exists {
			return NEXT, fmt.Errorf("slot id not provided")
		}

		slot, err := env.db.BandwidthSlotStore.Read(slotId)
		if err != nil {
			return NEXT, err
		}

		_, err = env.db.UserStore.Read(userId)
		if err != nil {
			return NEXT, err
		}

		for {
			fmt.Printf("Enter mac address: ")
			text, err := GetInput(env.in)
			if err != nil {
				return NEXT, err
			}
			if !IsValidMacAddress(text) {
				fmt.Println("Invalid mac address. Try again")
				continue
			}
			mac := text

			fmt.Printf("Enter alias: ")
			text, err = GetInput(env.in)
			if err != nil {
				return NEXT, err
			}

			ipAddress, err := env.router.GetUnusedIPAddress(slot.RemoteId)
			if err != nil {
				return NEXT, err
			}

			client, err := tplinkapi.NewClient(ipAddress, mac)
			if err != nil {
				return NEXT, err
			}
			// if client.IsMulticast() {
			// 	return NEXT, fmt.Errorf("multicast addresses not allowed")
			// }

			err = env.router.service.MakeIpAddressReservation(client)
			if err != nil {
				return NEXT, err
			}
			fmt.Printf("device assigned ip '%s'\n", client.IP)

			alias := text
			existingDevices, err := env.db.DeviceStore.ReadManyByMac([]string{client.Mac})
			if err != nil {
				return NEXT, err
			}

			if len(existingDevices) == 0 {
				device := storage.Device{
					UserId: userId,
					Mac:    client.Mac,
					Alias:  alias,
				}

				err = env.db.DeviceStore.Create(&device)
				if err != nil {
					return NEXT, err
				}
				fmt.Printf("Device added successfully %+v\n", device)
			} else {
				fmt.Println("Device already registered")
			}

			break
		}
		return NEXT, nil
	},
}

var ActionDeregisterDevice = &Action{
	Name:            "Deregister device",
	RequiresContext: []string{"deviceId"},
	Action: func(env *Env) (Navigation, error) {
		deviceId, exists := env.ctx["deviceId"]
		if !exists {
			return NEXT, fmt.Errorf("device id not provided")
		}

		device, err := env.db.DeviceStore.Read(deviceId)
		if err != nil {
			return NEXT, err
		}

		err = env.router.service.DeleteIpAddressReservation(device.Mac)
		if err != nil {
			return NEXT, err
		}

		err = env.db.DeviceStore.Delete(deviceId)
		if err != nil {
			return NEXT, err
		}

		fmt.Println("Device deregistered")
		delete(env.ctx, "deviceId")
		return BACK, nil
	},
}

var RootActionManageInternetAccess = &Action{
	Name: "Manage internet access",
	Children: []*Action{
		ActionShowConnectedDevices,
		ActionListBlockedDevices,
		ActionBlockDevice,
		ActionUnblockDevice,
	},
}

var ActionListBlockedDevices = &Action{
	Name: "Show blocked devices",
	Action: func(env *Env) (Navigation, error) {
		addresses, err := env.router.GetBlockedDevices()
		if err != nil {
			return NEXT, err
		}

		if len(addresses) == 0 {
			fmt.Println("no blocked devices found")
			return NEXT, nil
		}

		devices, err := env.db.DeviceStore.ReadManyByMac(addresses)
		if err != nil {
			return NEXT, err
		}

		fmt.Println("Blocked devices:")
		dataRows := make([][]string, len(devices))
		for i, device := range devices {
			user, err := device.GetUser(env.db.UserStore)
			var details string
			if err != nil {
				details = device.Alias
			} else {
				details = fmt.Sprintf("%s\t\t%s", device.Alias, user.Name)
			}
			dataRows[i] = []string{device.Alias, device.Mac, details}
		}
		err = PrintTable(env.out, dataRows, true, 3)
		return NEXT, err
	},
}

var ActionBlockDevice = &Action{
	Name: "Block device",
	Action: func(env *Env) (Navigation, error) {
		fmt.Printf("Enter devic mac address: ")
		mac, err := GetInput(env.in)
		if err != nil {
			return NEXT, nil
		}
		mac = strings.ToUpper(mac)
		if !IsValidMacAddress(mac) {
			fmt.Println("invalid mac address")
			return NEXT, nil
		}

		err = env.router.BlockDevice(mac)
		return NEXT, err
	},
}

var ActionUnblockDevice = &Action{
	Name: "Unblock device",
	Action: func(env *Env) (Navigation, error) {
		fmt.Printf("Enter devic mac address: ")
		mac, err := GetInput(env.in)
		if err != nil {
			return NEXT, nil
		}
		mac = strings.ToUpper(mac)
		if !IsValidMacAddress(mac) {
			fmt.Println("invalid mac address")
			return NEXT, nil
		}

		err = env.router.UnblockDevice(mac)
		if err != nil {
			return NEXT, err
		}

		fmt.Printf("device '%s' unblocked", mac)
		return NEXT, err
	},
}

var ActionQuit = &Action{
	Name: "Quit",
	Action: func(env *Env) (Navigation, error) {
		return NEXT, nil
	},
}

func RunMenuActions(env *Env, actions []*Action) (Navigation, error) {
	if QuitProgram(env.ctx) {
		return BACK, nil
	}

	var (
		options      strings.Builder
		navigation   Navigation
		containsQuit bool = false
	)
	for i, action := range actions {
		id := strconv.Itoa(i + 1)
		if action == ActionQuit {
			containsQuit = true
			id = "Q"
		}
		options.WriteString(
			fmt.Sprintf("%s: %s\n", id, action.Name),
		)
	}
	if !containsQuit {
		options.WriteString("B: Back\n")
		options.WriteString("Q: Quit\n")
	}

	for {
		fmt.Printf("\nChoose an action: \n%s\n\nChoice: ", options.String())
		choice, err := GetChoiceInput(env.in, len(actions))
		if err != nil {
			if err == ErrInvalidChoice || err == ErrInvalidInput {
				fmt.Printf("%v, try again\n", err)
				continue
			} else {
				return NEXT, err
			}
		}

		if choice == ExitChoice {
			break
		}

		if choice == QuitChoice {
			env.ctx.Set("quit", 1)
			break
		}

		action := actions[choice]
		if action == ActionQuit {
			env.ctx.Set("quit", 1)
			break
		}

		if action.Action != nil {
			navigation, err = action.Action(env)
			if err != nil {
				return NEXT, err
			}

			if navigation == BACK {
				break
			}

			if navigation == REPEAT {
				continue
			}
		}

		children := action.GetValidChildren(env.ctx)
		if len(children) > 0 {
			navigation, err = RunMenuActions(env, children)
			if QuitProgram(env.ctx) {
				break
			}

			if err != nil {
				return NEXT, err
			}

			if navigation == BACK {
				break
			}
		}
	}
	return NEXT, nil
}

func QuitProgram(ctx Context) bool {
	quit := ctx["quit"]
	return quit > 0
}

func ExportBindings(bindings []tplinkapi.ClientReservation, filename string) error {
	sort.Slice(bindings, func(i, j int) bool {
		return bindings[i].IpAsInt() < bindings[j].IpAsInt()
	})

	csvData := make([][]string, len(bindings)+1)
	headers := []string{"Mac", "IP", "Enabled"}
	csvData[0] = headers

	for i, binding := range bindings {
		enabled := "n"
		if binding.Enabled {
			enabled = "y"
		}

		csvData[i+1] = []string{binding.Mac, binding.IP, enabled}
	}

	if err := WriteToCsv(filename, csvData); err != nil {
		return err
	}

	fmt.Printf("saved to '%s'\n", filename)
	return nil
}
