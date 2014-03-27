// +build linux
package platform

/*
#include <linux/kvm.h>

// IOCTL calls.
const int GetApiVersion = KVM_GET_API_VERSION;
const int CreateVm = KVM_CREATE_VM;
const int GetVcpuMmapSize = KVM_GET_VCPU_MMAP_SIZE;
*/
import "C"

import (
    "log"
    "syscall"
)

type Vm struct {
    // The VM fd.
    fd  int

    // The next vcpu id to create.
    next_id int32

    // The next memory region slot to create.
    // This is not serialized because we will
    // recreate all regions (and the ordering
    // may even be different the 2nd time round).
    mem_region int

    // Our cpuid data.
    // At the moment, we just expose the full
    // host flags to the guest.
    cpuid []Cpuid

    // Our MSRs.
    msrs []uint32

    // The size of the vcpu mmap structs.
    mmap_size int

    // Eventfds are enabled?
    use_eventfds bool

    // Our vcpus.
    vcpus []*Vcpu
}

func getMmapSize(fd int) (int, error) {
    // Get the size of the Mmap structure.
    r, _, e := syscall.Syscall(
        syscall.SYS_IOCTL,
        uintptr(fd),
        uintptr(C.GetVcpuMmapSize),
        0)
    if e != 0 {
        return 0, e
    }
    return int(r), nil
}

func NewVm() (*Vm, error) {
    fd, err := syscall.Open("/dev/kvm", syscall.O_RDWR|syscall.O_CLOEXEC, 0)
    if err != nil {
        return nil, err
    }
    defer syscall.Close(fd)

    // Check API version.
    version, _, e := syscall.Syscall(
        syscall.SYS_IOCTL,
        uintptr(fd),
        uintptr(C.GetApiVersion),
        0)
    if version != 12 || e != 0 {
        return nil, e
    }

    // Check our extensions.
    for _, capSpec := range requiredCapabilities {
        err = checkCapability(fd, capSpec)
        if err != nil {
            return nil, err
        }
    }

    // Make sure we have the mmap size.
    mmap_size, err := getMmapSize(fd)
    if err != nil {
        return nil, err
    }

    // Make sure we have cpuid data.
    cpuid, err := defaultCpuid(fd)
    if err != nil {
        return nil, err
    }

    // Get our list of available MSRs.
    msrs, err := availableMsrs(fd)
    if err != nil {
        return nil, err
    }

    // Create new VM.
    vmfd, _, e := syscall.Syscall(
        syscall.SYS_IOCTL,
        uintptr(fd),
        uintptr(C.CreateVm),
        0)
    if e != 0 {
        return nil, e
    }

    // Make sure this VM gets closed.
    // (Same thing is done for Vcpus).
    syscall.CloseOnExec(int(vmfd))

    // Prepare our VM object.
    log.Print("kvm: VM created.")
    vm := &Vm{
        fd:        int(vmfd),
        vcpus:     make([]*Vcpu, 0, 0),
        cpuid:     cpuid,
        msrs:      msrs,
        mmap_size: mmap_size,
    }

    return vm, nil
}

func (vm *Vm) Dispose() error {
    return syscall.Close(vm.fd)
}

func (vm *Vm) Vcpus() []*Vcpu {
    return vm.vcpus
}

func (vm *Vm) VcpuInfo() ([]VcpuInfo, error) {

    vcpus := make([]VcpuInfo, 0, len(vm.vcpus))
    for _, vcpu := range vm.vcpus {
        vcpuinfo, err := NewVcpuInfo(vcpu)
        if err != nil {
            return nil, err
        }

        vcpus = append(vcpus, vcpuinfo)
    }

    return vcpus, nil
}
