package sensor

import (
	"encoding/json"
	"errors"
	"fmt"
	"sensor/count"
	"sync"
	"time"
)

/**
 * @return 关联至attachIP上的至少0个传感器
 */
func (dl *LocalDeviceDetail) GetLocalSensorList(attachIP string) []*LocalSensorInformation {
	var ret []*LocalSensorInformation
	for _, v := range dl.LocalSensorInformation {
		if v.Attach == attachIP {
			ret = append(ret, v)
		}
	}
	return ret
}

func (ls *LocalSensorInformation) StartSensorMeasureTask() {

}

type TaskSensorKey struct {
	Addr   byte   // 设备地址
	Attach string // 附着设备Gateway
	Type   byte   // 指令类型
	// Interval int    // 最小间隔时间(如果测量没有被阻塞的话将会接近的最小间隔时间, 阻塞的原因可能归结于透传设备无法响应请求指令)
}

type TaskSensorBody struct {
	// TaskSensorKey TaskSensorKey // 任务唯一id
	Type           byte   // 指令类型
	RequestData    []byte // 生成的指令数据
	SensorID       string // 传感器ID
	SensorAddr     byte   // 传感器地址
	SensorAttachIP string // 传感器依附IP

	customFunction func(body TaskSensorBody, wg *sync.WaitGroup)
}

var tw *TimeWheel

const taskSecond int64 = 1000000000

const (
	DissolvedOxygenAndTemperature = iota // 溶氧量
	D2
	D3
	D4
	D5
	D6
	D7
	D8
	// 自定义指令Type 用于一次性的用户设置指令等

)

//// 指令类型Type
//const DissolvedOxygenAndTemperature byte = 0x01 // 溶氧量和温度
//const D2 byte = 0x02                            // 未定义的类型
//const D3 byte = 0x04                            // ..
//const D4 byte = 0x08                            // ..
//const D5 byte = 0x10                            // ..
//const D6 byte = 0x20                            // ..
//const D7 byte = 0x40                            // ..
//const D8 byte = 0x80                            // 未定义的类型

// 自定义指令Type 用于一次性的用户设置指令等

// enum
// ...
// ...

/**
 * 测量请求体创建
 */
func (ts *TaskSensorBody) CreateMeasureRequest() {
	var sr []byte
	// 设备ADDR
	sr = append(sr, ts.SensorAddr)
	// 指令功能码
	sr = append(sr, InfoMK["ReadFunc"]...)
	// 寄存器地址和数量
	sr = append(sr, InfoMK["RMeasure"]...)
	// CRC_ModBus
	sr = append(sr, CreateCRC(sr)...)
	ts.RequestData = sr
}

/**
 * 默认处理过程
 * 当LocalSensorInformation没有设置handler时, 所调用的默认处理过程
 * DefaultHandler中规定了几种默认的处理方式
 */
func DefaultSensorHandler(body TaskSensorBody, wg *sync.WaitGroup) {
	// 传感器异常
	if count.IsForbidden(body.SensorID) {
		wg.Done()
		return
	}

	// 关闭
	ls, _ := GetLocalSensor(body.SensorID)
	if ls.IsClosed() {
		wg.Done()
		return
	}

	//if value, err := GetLocalSensor(body.SensorID); err != nil {
	//	fmt.Println("[FAIL] key not found ", err)
	//} else if value.Status == STATUS_DETACH || value.Status == STATUS_CLOSED {
	//	// 不执行操作
	//	wg.Done()
	//	return
	//}

	switch body.Type {
	case DissolvedOxygenAndTemperature:
		// 得到透传conn
		b, _ := GetDeviceSession(body.SensorAttachIP)
		// 合成地址
		body.CreateMeasureRequest()
		fmt.Printf("[INFO] 测量请求 ID:%s 设备地址:%d 任务类型:%d 请求数据:%b\n", body.SensorID, body.SensorAddr, body.Type, body.RequestData)
		// 向传感器发送对应测量请求
		p, err := b.MeasureRequest(body.RequestData, []string{"Oxygen", "Temp"})
		if err != nil {
			fmt.Println("[FAIL] 请求失败")
			// TODO 超时处理
			v, _ := GetLocalSensor(body.SensorID)
			// 超时标记
			if count.AddErrorOperation(body.SensorID) > 3 {
				v.Status = STATUS_DETACH
			}
			fmt.Printf("[WARN] 查询错误 ID:%s 发生第%d次错误 恢复时间: %s\n", body.SensorID, count.GetErrorCount(body.SensorID), count.GetRetryTime(body.SensorID).Format("2006/1/2 15:04:05"))
			// v.Status = STATUS_DETACH
			// waitGroup完成

			break
		}
		p.SensorID = body.SensorID
		send, err := json.Marshal(p)
		client, _ := GetMQTTInstance()
		client.Publish("sensor/oxygen/measure", 1, false, send)
		break
	case D2:
		// TODO
		fmt.Println("d2")
		break
	case D3:
		// TODO
		fmt.Println("d3")
		break
	case D4:
		// TODO
		fmt.Println("d4")
		break
	case D5:
		// TODO
		fmt.Println("d5")
		break
	case D6:
		// TODO
		fmt.Println("d6")
		break
	case D7:
		// TODO
		fmt.Println("d7")
		break
	case D8:
		// TODO
		fmt.Println("d8")
		break
	default:
		fmt.Println("default")
	}
	wg.Done()
}

/**
 * 使用自定义Handler以代替默认处理过程
 */
func (ls *LocalSensorInformation) AddTaskHandler(callback func(body TaskSensorBody, wg *sync.WaitGroup)) {
	ls.TaskHandler = callback
}

/**
 * 移除自定义Handler
 */
func (ls *LocalSensorInformation) RemoveTaskHandler() bool {
	if ls.TaskHandler == nil {
		return false
	}
	ls.TaskHandler = nil
	return true
}

/**
 * 启动传感器任务
 * @param ls.interval 测量间隔时间
 * @param times 指定任务次数
 * @param queueChannel 单DTU内任务的阻塞队列
 * -1 -> 无限次
 * >1 -> 有限次
 * @return error 错误的添加会触发
 */
func (ls *LocalSensorInformation) CreateTask(times int, queueChannel chan TaskSensorBody) error {
	key := TaskSensorKey{ls.Addr, ls.Attach, ls.Type}
	// key由传感器地址addr + 依附下位机attachIP + 传感器类型type构成
	body := TaskSensorBody{}
	body.SensorAddr = ls.Addr
	body.Type = ls.Type
	body.RequestData = nil
	body.SensorID = ls.SensorID
	body.SensorAttachIP = ls.Attach

	// 是否存在自定义任务
	// 这里应该不需要这个自定义任务了, 应该改到pop中
	//if ls.TaskHandler == nil {
	//	body.customFunction = nil
	//} else {
	//	body.customFunction = ls.TaskHandler
	//}

	// data由信息体data + 阻塞channel构成
	data := TaskData{"Data": body, "Channel": queueChannel}
	return GetTimeWheel().AddTask(time.Duration(ls.Interval*taskSecond), times, key, data, TaskSensorPush)
}

/**
 * 单DTU任务阻塞队列的压入
 * @param queueChannel 单DTU内任务的阻塞队列
 */
func TaskSensorPush(data TaskData) {
	body := data["Data"].(TaskSensorBody)
	queueChannel := data["Channel"].(chan TaskSensorBody)
	defer func() {
		if recover() != nil {
			fmt.Println("[INFO] 通道已关闭, 尝试再次关闭任务")
			key := TaskSensorKey{body.SensorAddr, body.SensorAttachIP, body.Type}
			if err := GetTimeWheel().RemoveTask(key); err != nil {
				fmt.Println("[WARN] 尝试失败, 不存在的任务Key")
			}
		}
	}()
	queueChannel <- body
}

/**
 * 单DTU任务调度Routine
 * 特别说明: 当遇到的任务不是定时执行的时候, 比如是用户修改了传感器的某一项参数时, 需要提前得知queueChannel的地址
 * @param queueChannel 单DTU内任务的阻塞队列
 */
func (ds *DeviceSession) TaskSensorPop(queueChannel chan TaskSensorBody) {
	var wg sync.WaitGroup

	for v := range queueChannel {
		wg.Add(1)
		DefaultSensorHandler(v, &wg)
		wg.Wait()
	}

	fmt.Println("[INFO] POP成功关闭")
}

/**
 * 移除传感器任务
 * @return error 错误的移除同样会触发
 */
func (ls *LocalSensorInformation) RemoveTask() error {
	key := TaskSensorKey{ls.Addr, ls.Attach, ls.Type}
	return GetTimeWheel().RemoveTask(key)
}

/**
 * 更新传感器任务
 * @param interval 测量间隔时间
 * @return error 需要注意多次提交相同key任务时会触发
 *
 */
func (ls *LocalSensorInformation) UpdateTask(interval time.Duration, queueChannel chan TaskSensorBody) error {
	key := TaskSensorKey{ls.Addr, ls.Attach, ls.Type}
	body := TaskSensorBody{}
	body.Type = ls.Type
	body.RequestData = nil
	body.SensorID = ls.SensorID
	if ls.TaskHandler == nil {
		body.customFunction = nil
	} else {
		body.customFunction = ls.TaskHandler
	}
	data := TaskData{"Data": body, "Channel": queueChannel}
	return GetTimeWheel().UpdateTask(key, interval, data)
}

/**
 * 初始化TimeWheel
 */
func TimeWheelInit() *TimeWheel {
	tw = New(time.Second, 180)
	tw.Start()
	return tw
}

/*
 * 获得TimeWheel单例
 */
func GetTimeWheel() *TimeWheel {
	if tw == nil {
		tw = New(time.Second, 180)
		tw.Start()
	}
	return tw
}

/*
 * 定时任务设置
 * @return ch 给processor进行回收
 */
func TaskSetup(attachIP string) chan TaskSensorBody {
	ch := make(chan TaskSensorBody, 10)
	// 这个pop每个dtu有且只有一个, 生命周期应与tcp挂钩
	ds, _ := GetDeviceSession(attachIP)
	go ds.TaskSensorPop(ch)
	// 此处得到attach到该dtu的至少0个, 至多3个传感器的参数

	// 为attach的每一个传感器设置定时任务
	for _, v := range GetLocalDevicesInstance().GetLocalSensorList(attachIP) {
		v.ScanSensorStatus()
		if err := v.CreateTask(-1, ch); err != nil {
			continue
		}
		fmt.Println("[INFO] 进入队列 ID:" + v.SensorID)
	}

	return ch
}

/*
 * 扫描attach(下位机)内传感器状态
 * 在processor内的for进行首次判断
 */
func (ls *LocalSensorInformation) ScanSensorStatus() {
	fmt.Println("[INFO] 等待连接 ID:" + ls.SensorID + " FROM " + ls.Attach)
	ds, _ := GetDeviceSession(ls.Attach)
	var sr []byte
	// 设备ADDR
	sr = append(sr, ls.Addr)
	// 指令功能码
	sr = append(sr, InfoMK["ReadFunc"]...)
	// 寄存器地址和数量
	sr = append(sr, InfoMK["RAddr"]...)
	// CRC_ModBus
	sr = append(sr, CreateCRC(sr)...)
	if _, err := ds.SendToSensor(sr); err != nil {
		// 超时
		ls.Status = STATUS_DETACH
		count.AddErrorOperationBan(ls.SensorID)
		fmt.Println("[WARN] 连接超时 ID:" + ls.SensorID + " FROM " + ls.Attach)
	} else {
		// TODO: 最后记得把fmt换成日志log输出
		ls.Status = STATUS_NORMAL
		count.ClsErrorCount(ls.SensorID)
		fmt.Println("[INFO] 连接成功 ID:" + ls.SensorID + " FROM " + ls.Attach)
	}
}

/*
 * 通过sensorID获得LocalSensorInformation
 */
func GetLocalSensor(sensorID string) (*LocalSensorInformation, error) {
	ins := GetLocalDevicesInstance().LocalSensorInformation
	for _, v := range ins {
		if v.SensorID == sensorID {
			return v, nil
		}
	}
	return nil, errors.New("not find sensorID for this device")
}

func (ls *LocalSensorInformation) IsClosed() bool {
	if ls.Status == STATUS_CLOSED {
		return true
	}
	return false
}

func (ls *LocalSensorInformation) Open() {
	ls.Status = STATUS_NORMAL
}

func (ls *LocalSensorInformation) Close() {
	ls.Status = STATUS_CLOSED
}

func (ls *LocalSensorInformation) Detach() {
	ls.Status = STATUS_DETACH
}
