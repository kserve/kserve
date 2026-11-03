
import triton
import triton.language as tl
from triton.compiler.compiler import AttrsDescriptor

from torch._inductor.runtime import triton_helpers, triton_heuristics
from torch._inductor.runtime.triton_helpers import libdevice, math as tl_math
from torch._inductor.runtime.hints import AutotuneHint, ReductionHint, TileHint, DeviceProperties
triton_helpers.set_driver_to_gpu()

@triton_heuristics.pointwise(
    size_hints={'x': 2097152},
    filename=__file__,
    triton_meta={'signature': {'in_ptr0': '*bf16', 'in_ptr1': '*i64', 'in_ptr2': '*bf16', 'out_ptr0': '*bf16', 'out_ptr1': '*bf16', 'xnumel': 'i32'}, 'device': DeviceProperties(type='hip', index=0, multi_processor_count=104, cc='gfx90a', major=9, regs_per_multiprocessor=65536, max_threads_per_multi_processor=2048, warp_size=64), 'constants': {}, 'configs': [AttrsDescriptor.from_dict({'arg_properties': {'tt.divisibility': (0, 1, 2, 3, 4, 5), 'tt.equal_to': ()}, 'cls': 'AttrsDescriptor'})]},
    inductor_meta={'grid_type': 'Grid1D', 'autotune_hints': set(), 'kernel_name': 'triton_poi_fused_cat_0', 'mutated_arg_names': [], 'optimize_mem': True, 'no_x_dim': False, 'num_load': 10, 'num_reduction': 0, 'backend_hash': '5F6849C846FE45386D7FD4995E383DFF233E57C8430A3EA7CA224D4D096E26A5', 'are_deterministic_algorithms_enabled': False, 'assert_indirect_indexing': True, 'autotune_local_cache': True, 'autotune_pointwise': True, 'autotune_remote_cache': None, 'force_disable_caches': False, 'dynamic_scale_rblock': True, 'max_autotune': False, 'max_autotune_pointwise': False, 'min_split_scan_rblock': 256, 'spill_threshold': 16, 'store_cubin': False, 'is_hip': True},
    min_elem_per_thread=0
)
@triton.jit
def triton_poi_fused_cat_0(in_ptr0, in_ptr1, in_ptr2, out_ptr0, out_ptr1, xnumel, XBLOCK : tl.constexpr):
    xoffset = tl.program_id(0) * XBLOCK
    xindex = xoffset + tl.arange(0, XBLOCK)[:]
    xmask = xindex < xnumel
    x0 = (xindex % 64)
    x1 = ((xindex // 64) % 16)
    x2 = xindex // 1024
    x4 = xindex
    tmp0 = x0
    tmp1 = tl.full([1], 0, tl.int64)
    tmp2 = tmp0 >= tmp1
    tmp3 = tl.full([1], 32, tl.int64)
    tmp4 = tmp0 < tmp3
    tmp5 = tmp4.to(tl.int1)
    tmp6 = tl.load(in_ptr0 + (64*x1 + 3072*x2 + (x0)), xmask & tmp5, eviction_policy='evict_last', other=0.0).to(tl.float32)
    tmp7 = tl.load(in_ptr1 + (x2), xmask & tmp5, eviction_policy='evict_last', other=0.0)
    tmp8 = tl.full([XBLOCK], 32768, tl.int32)
    tmp9 = tmp7 + tmp8
    tmp10 = tmp7 < 0
    tmp11 = tl.where(tmp10, tmp9, tmp7)
    tl.device_assert(((0 <= tl.broadcast_to(tmp11, [XBLOCK])) & (tl.broadcast_to(tmp11, [XBLOCK]) < 32768)) | ~(xmask & tmp5), "index out of bounds: 0 <= tl.broadcast_to(tmp11, [XBLOCK]) < 32768")
    tmp13 = tl.load(in_ptr2 + (64*tmp11 + (x0)), xmask & tmp5, eviction_policy='evict_last', other=0.0).to(tl.float32)
    tmp14 = tmp6 * tmp13
    tmp15 = tl.load(in_ptr0 + (32 + 64*x1 + 3072*x2 + (x0)), xmask & tmp5, eviction_policy='evict_last', other=0.0).to(tl.float32)
    tmp16 = tl.load(in_ptr2 + (32 + 64*tmp11 + (x0)), xmask & tmp5, eviction_policy='evict_last', other=0.0).to(tl.float32)
    tmp17 = tmp15 * tmp16
    tmp18 = tmp14 - tmp17
    tmp19 = tl.full(tmp18.shape, 0.0, tmp18.dtype)
    tmp20 = tl.where(tmp5, tmp18, tmp19)
    tmp21 = tmp0 >= tmp3
    tmp22 = tl.full([1], 64, tl.int64)
    tmp23 = tmp0 < tmp22
    tmp24 = tmp21.to(tl.int1)
    tmp25 = tl.load(in_ptr0 + (32 + 64*x1 + 3072*x2 + ((-32) + x0)), xmask & tmp24, eviction_policy='evict_last', other=0.0).to(tl.float32)
    tmp26 = tl.load(in_ptr1 + (x2), xmask & tmp24, eviction_policy='evict_last', other=0.0)
    tmp27 = tl.full([XBLOCK], 32768, tl.int32)
    tmp28 = tmp26 + tmp27
    tmp29 = tmp26 < 0
    tmp30 = tl.where(tmp29, tmp28, tmp26)
    tl.device_assert(((0 <= tl.broadcast_to(tmp30, [XBLOCK])) & (tl.broadcast_to(tmp30, [XBLOCK]) < 32768)) | ~(xmask & tmp24), "index out of bounds: 0 <= tl.broadcast_to(tmp30, [XBLOCK]) < 32768")
    tmp32 = tl.load(in_ptr2 + (64*tmp30 + ((-32) + x0)), xmask & tmp24, eviction_policy='evict_last', other=0.0).to(tl.float32)
    tmp33 = tmp25 * tmp32
    tmp34 = tl.load(in_ptr0 + (64*x1 + 3072*x2 + ((-32) + x0)), xmask & tmp24, eviction_policy='evict_last', other=0.0).to(tl.float32)
    tmp35 = tl.load(in_ptr2 + (32 + 64*tmp30 + ((-32) + x0)), xmask & tmp24, eviction_policy='evict_last', other=0.0).to(tl.float32)
    tmp36 = tmp34 * tmp35
    tmp37 = tmp33 + tmp36
    tmp38 = tl.full(tmp37.shape, 0.0, tmp37.dtype)
    tmp39 = tl.where(tmp24, tmp37, tmp38)
    tmp40 = tl.where(tmp4, tmp20, tmp39)
    tmp41 = tl.load(in_ptr0 + (1024 + 64*x1 + 3072*x2 + (x0)), xmask & tmp5, eviction_policy='evict_last', other=0.0).to(tl.float32)
    tmp42 = tmp41 * tmp13
    tmp43 = tl.load(in_ptr0 + (1056 + 64*x1 + 3072*x2 + (x0)), xmask & tmp5, eviction_policy='evict_last', other=0.0).to(tl.float32)
    tmp44 = tmp43 * tmp16
    tmp45 = tmp42 - tmp44
    tmp46 = tl.full(tmp45.shape, 0.0, tmp45.dtype)
    tmp47 = tl.where(tmp5, tmp45, tmp46)
    tmp48 = tl.load(in_ptr0 + (1056 + 64*x1 + 3072*x2 + ((-32) + x0)), xmask & tmp24, eviction_policy='evict_last', other=0.0).to(tl.float32)
    tmp49 = tmp48 * tmp32
    tmp50 = tl.load(in_ptr0 + (1024 + 64*x1 + 3072*x2 + ((-32) + x0)), xmask & tmp24, eviction_policy='evict_last', other=0.0).to(tl.float32)
    tmp51 = tmp50 * tmp35
    tmp52 = tmp49 + tmp51
    tmp53 = tl.full(tmp52.shape, 0.0, tmp52.dtype)
    tmp54 = tl.where(tmp24, tmp52, tmp53)
    tmp55 = tl.where(tmp4, tmp47, tmp54)
    tl.store(out_ptr0 + (x4), tmp40, xmask)
    tl.store(out_ptr1 + (x4), tmp55, xmask)
