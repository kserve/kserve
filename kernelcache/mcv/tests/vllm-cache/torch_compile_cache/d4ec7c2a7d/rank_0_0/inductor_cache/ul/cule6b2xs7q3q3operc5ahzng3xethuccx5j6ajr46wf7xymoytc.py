# AOT ID: ['24_inference']
from ctypes import c_void_p, c_long, c_int
import torch
import math
import random
import os
import tempfile
from math import inf, nan
from cmath import nanj
from torch._inductor.hooks import run_intermediate_hooks
from torch._inductor.utils import maybe_profile
from torch._inductor.codegen.memory_planning import _align as align
from torch import device, empty_strided
from torch._inductor.async_compile import AsyncCompile
from torch._inductor.select_algorithm import extern_kernels
from torch._inductor.codegen.multi_kernel import MultiKernelCall

aten = torch.ops.aten
inductor_ops = torch.ops.inductor
_quantized = torch.ops._quantized
assert_size_stride = torch._C._dynamo.guards.assert_size_stride
empty_strided_cpu = torch._C._dynamo.guards._empty_strided_cpu
empty_strided_cuda = torch._C._dynamo.guards._empty_strided_cuda
empty_strided_xpu = torch._C._dynamo.guards._empty_strided_xpu
reinterpret_tensor = torch._C._dynamo.guards._reinterpret_tensor
alloc_from_pool = torch.ops.inductor._alloc_from_pool
async_compile = AsyncCompile()
empty_strided_p2p = torch._C._distributed_c10d._SymmetricMemory.empty_strided_p2p


async_compile.wait(globals())
del async_compile

def call(args):
    arg0_1, arg1_1, arg2_1, arg3_1, arg4_1, arg5_1, arg6_1, arg7_1 = args
    args.clear()
    s0 = arg1_1
    assert_size_stride(arg0_1, (s0, 16, 64), (1024, 64, 1))
    assert_size_stride(arg2_1, (1024, 1024), (1024, 1))
    assert_size_stride(arg3_1, (1024, ), (1, ))
    assert_size_stride(arg4_1, (s0, 1024), (1024, 1))
    assert_size_stride(arg5_1, (5632, 1024), (1024, 1))
    assert_size_stride(arg6_1, (1024, 2816), (2816, 1))
    assert_size_stride(arg7_1, (1024, ), (1, ))
    with torch.cuda._DeviceGuard(0):
        torch.cuda.set_device(0)
        buf0 = empty_strided_cuda((s0, 1024), (1024, 1), torch.bfloat16)
        # Topologically Sorted Source Nodes: [linear], Original ATen: [aten.mm]
        extern_kernels.mm(reinterpret_tensor(arg0_1, (s0, 1024), (1024, 1), 0), reinterpret_tensor(arg2_1, (1024, 1024), (1, 1024), 0), out=buf0)
        del arg0_1
        del arg2_1
        # Topologically Sorted Source Nodes: [], Original ATen: []
        torch.ops._C.fused_add_rms_norm.default(input=buf0, residual=arg4_1, weight=arg3_1, epsilon=1e-06)
        del arg3_1
        buf4 = empty_strided_cuda((s0, 2816), (2816, 1), torch.bfloat16)
        buf5 = empty_strided_cuda((s0, 5632), (5632, 1), torch.bfloat16)
        # Topologically Sorted Source Nodes: [linear_1], Original ATen: [aten.mm]
        extern_kernels.mm(buf0, reinterpret_tensor(arg5_1, (1024, 5632), (1, 1024), 0), out=buf5)
        del arg5_1
        # Topologically Sorted Source Nodes: [], Original ATen: []
        torch.ops._C.silu_and_mul.default(buf4, buf5)
        del buf5
        buf8 = buf0; del buf0  # reuse
        # Topologically Sorted Source Nodes: [linear_2], Original ATen: [aten.mm]
        extern_kernels.mm(buf4, reinterpret_tensor(arg6_1, (2816, 1024), (1, 2816), 0), out=buf8)
        del arg6_1
        del buf4
        # Topologically Sorted Source Nodes: [], Original ATen: []
        torch.ops._C.fused_add_rms_norm.default(input=buf8, residual=arg4_1, weight=arg7_1, epsilon=1e-06)
        del arg4_1
        del arg7_1
    return (buf8, )


def benchmark_compiled_module(times=10, repeat=10):
    from torch._dynamo.testing import rand_strided
    from torch._inductor.utils import print_performance
    arg0_1 = rand_strided((2048, 16, 64), (1024, 64, 1), device='cuda:0', dtype=torch.bfloat16)
    arg1_1 = 2048
    arg2_1 = rand_strided((1024, 1024), (1024, 1), device='cuda:0', dtype=torch.bfloat16)
    arg3_1 = rand_strided((1024, ), (1, ), device='cuda:0', dtype=torch.bfloat16)
    arg4_1 = rand_strided((2048, 1024), (1024, 1), device='cuda:0', dtype=torch.bfloat16)
    arg5_1 = rand_strided((5632, 1024), (1024, 1), device='cuda:0', dtype=torch.bfloat16)
    arg6_1 = rand_strided((1024, 2816), (2816, 1), device='cuda:0', dtype=torch.bfloat16)
    arg7_1 = rand_strided((1024, ), (1, ), device='cuda:0', dtype=torch.bfloat16)
    fn = lambda: call([arg0_1, arg1_1, arg2_1, arg3_1, arg4_1, arg5_1, arg6_1, arg7_1])
    return print_performance(fn, times=times, repeat=repeat)


if __name__ == "__main__":
    from torch._inductor.wrapper_benchmark import compiled_module_main
    compiled_module_main('None', benchmark_compiled_module)
