def forward(self, input_ids, positions, intermediate_tensors, inputs_embeds):
    tmp_0 = None
    tmp_1 = None
    tmp_2 = __import_torch_dot__dynamo_dot_utils.call_size
    tmp_3 = __compiled_fn_1
    tmp_4 = None
    tmp_5 = tmp_4
    tmp_6 = tmp_4
    tmp_7 = tmp_4
    tmp_8 = tmp_0.dict_getitem
    tmp_9 = tmp_0.dict_getitem
    tmp_10 = tmp_0.dict_getitem
    __temp_3 = tmp_0.dict_getitem(tmp_4(self._modules, 'embed_tokens').
        _parameters, 'weight')
    tmp_11 = tmp_2
    tmp_12 = tmp_4
    tmp_13 = tmp_4
    tmp_14 = tmp_4
    tmp_15 = tmp_4
    tmp_16 = tmp_4
    tmp_17 = tmp_4
    tmp_18 = tmp_4
    tmp_19 = tmp_4
    tmp_20 = tmp_4
    __temp_7 = tmp_4(tmp_4(tmp_4(tmp_4(tmp_7, 'layers')._modules, '0')._modules,
        'input_layernorm')._parameters, 'weight')
    tmp_21 = __temp_3
    tmp_22 = tmp_4
    tmp_23 = tmp_4
    tmp_24 = tmp_4
    tmp_25 = tmp_4
    tmp_26 = tmp_4
    tmp_27 = tmp_4
    __temp_10 = tmp_4(tmp_4(tmp_4(tmp_17, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_28 = __temp_7
    __temp_11 = tmp_4(tmp_27, 'weight')
    tmp_29 = __temp_10
    tmp_30 = tmp_1
    tmp_31 = __temp_11
    tmp_32 = tmp_4
    tmp_33 = tmp_4
    tmp_34 = tmp_4
    __temp_14 = tmp_4(tmp_4(tmp_24, 'rotary_emb')._buffers, 'cos_sin_cache')
    tmp_35 = tmp_30
    tmp_36 = tmp_4
    tmp_37 = tmp_4
    tmp_38 = tmp_4
    __temp_16 = tmp_4(tmp_4(tmp_24, 'o_proj')._parameters, 'weight')
    tmp_39 = __temp_14
    tmp_40 = tmp_4
    tmp_41 = tmp_4
    tmp_42 = tmp_4
    __temp_18 = tmp_4(tmp_4(tmp_17, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_43 = __temp_16
    tmp_44 = tmp_4
    tmp_45 = tmp_4
    tmp_46 = tmp_4
    tmp_47 = tmp_4
    tmp_48 = tmp_4
    tmp_49 = tmp_4
    __temp_21 = tmp_4(tmp_4(tmp_4(tmp_17, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_50 = __temp_18
    tmp_51 = tmp_4
    tmp_52 = tmp_4
    tmp_53 = tmp_4
    __temp_23 = tmp_4(tmp_4(tmp_46, 'down_proj')._parameters, 'weight')
    tmp_54 = __temp_21
    tmp_55 = tmp_4
    tmp_56 = tmp_4
    tmp_57 = tmp_4
    tmp_58 = tmp_4
    tmp_59 = tmp_4
    tmp_60 = tmp_4
    __temp_26 = tmp_4(tmp_4(tmp_4(tmp_14, '1')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_61 = __temp_23
    tmp_62 = tmp_4
    tmp_63 = tmp_4
    tmp_64 = tmp_4
    tmp_65 = tmp_4
    tmp_66 = tmp_4
    tmp_67 = tmp_4
    __temp_29 = tmp_4(tmp_4(tmp_4(tmp_57, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_68 = __temp_26
    __temp_30 = tmp_4(tmp_67, 'weight')
    tmp_69 = __temp_29
    tmp_70 = tmp_4
    tmp_71 = tmp_4
    tmp_72 = tmp_4
    __temp_32 = tmp_4(tmp_4(tmp_64, 'o_proj')._parameters, 'weight')
    tmp_73 = __temp_30
    tmp_74 = tmp_4
    tmp_75 = tmp_4
    tmp_76 = tmp_4
    __temp_34 = tmp_4(tmp_4(tmp_57, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_77 = __temp_32
    tmp_78 = tmp_4
    tmp_79 = tmp_4
    tmp_80 = tmp_4
    tmp_81 = tmp_4
    tmp_82 = tmp_4
    tmp_83 = tmp_4
    __temp_37 = tmp_4(tmp_4(tmp_4(tmp_57, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_84 = __temp_34
    tmp_85 = tmp_4
    tmp_86 = tmp_4
    tmp_87 = tmp_4
    __temp_39 = tmp_4(tmp_4(tmp_80, 'down_proj')._parameters, 'weight')
    tmp_88 = __temp_37
    tmp_89 = tmp_4
    tmp_90 = tmp_4
    tmp_91 = tmp_4
    tmp_92 = tmp_4
    tmp_93 = tmp_4
    tmp_94 = tmp_4
    __temp_42 = tmp_4(tmp_4(tmp_4(tmp_14, '2')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_95 = __temp_39
    tmp_96 = tmp_4
    tmp_97 = tmp_4
    tmp_98 = tmp_4
    tmp_99 = tmp_4
    tmp_100 = tmp_4
    tmp_101 = tmp_4
    __temp_45 = tmp_4(tmp_4(tmp_4(tmp_91, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_102 = __temp_42
    __temp_46 = tmp_4(tmp_101, 'weight')
    tmp_103 = __temp_45
    tmp_104 = tmp_4
    tmp_105 = tmp_4
    tmp_106 = tmp_4
    __temp_48 = tmp_4(tmp_4(tmp_98, 'o_proj')._parameters, 'weight')
    tmp_107 = __temp_46
    tmp_108 = tmp_4
    tmp_109 = tmp_4
    tmp_110 = tmp_4
    __temp_50 = tmp_4(tmp_4(tmp_91, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_111 = __temp_48
    tmp_112 = tmp_4
    tmp_113 = tmp_4
    tmp_114 = tmp_4
    tmp_115 = tmp_4
    tmp_116 = tmp_4
    tmp_117 = tmp_4
    __temp_53 = tmp_4(tmp_4(tmp_4(tmp_91, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_118 = __temp_50
    tmp_119 = tmp_4
    tmp_120 = tmp_4
    tmp_121 = tmp_4
    __temp_55 = tmp_4(tmp_4(tmp_114, 'down_proj')._parameters, 'weight')
    tmp_122 = __temp_53
    tmp_123 = tmp_4
    tmp_124 = tmp_4
    tmp_125 = tmp_4
    tmp_126 = tmp_4
    tmp_127 = tmp_4
    tmp_128 = tmp_4
    __temp_58 = tmp_4(tmp_4(tmp_4(tmp_14, '3')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_129 = __temp_55
    tmp_130 = tmp_4
    tmp_131 = tmp_4
    tmp_132 = tmp_4
    tmp_133 = tmp_4
    tmp_134 = tmp_4
    tmp_135 = tmp_4
    __temp_61 = tmp_4(tmp_4(tmp_4(tmp_125, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_136 = __temp_58
    __temp_62 = tmp_4(tmp_135, 'weight')
    tmp_137 = __temp_61
    tmp_138 = tmp_4
    tmp_139 = tmp_4
    tmp_140 = tmp_4
    __temp_64 = tmp_4(tmp_4(tmp_132, 'o_proj')._parameters, 'weight')
    tmp_141 = __temp_62
    tmp_142 = tmp_4
    tmp_143 = tmp_4
    tmp_144 = tmp_4
    __temp_66 = tmp_4(tmp_4(tmp_125, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_145 = __temp_64
    tmp_146 = tmp_4
    tmp_147 = tmp_4
    tmp_148 = tmp_4
    tmp_149 = tmp_4
    tmp_150 = tmp_4
    tmp_151 = tmp_4
    __temp_69 = tmp_4(tmp_4(tmp_4(tmp_125, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_152 = __temp_66
    tmp_153 = tmp_4
    tmp_154 = tmp_4
    tmp_155 = tmp_4
    __temp_71 = tmp_4(tmp_4(tmp_148, 'down_proj')._parameters, 'weight')
    tmp_156 = __temp_69
    tmp_157 = tmp_4
    tmp_158 = tmp_4
    tmp_159 = tmp_4
    tmp_160 = tmp_4
    tmp_161 = tmp_4
    tmp_162 = tmp_4
    __temp_74 = tmp_4(tmp_4(tmp_4(tmp_14, '4')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_163 = __temp_71
    tmp_164 = tmp_4
    tmp_165 = tmp_4
    tmp_166 = tmp_4
    tmp_167 = tmp_4
    tmp_168 = tmp_4
    tmp_169 = tmp_4
    __temp_77 = tmp_4(tmp_4(tmp_4(tmp_159, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_170 = __temp_74
    __temp_78 = tmp_4(tmp_169, 'weight')
    tmp_171 = __temp_77
    tmp_172 = tmp_4
    tmp_173 = tmp_4
    tmp_174 = tmp_4
    __temp_80 = tmp_4(tmp_4(tmp_166, 'o_proj')._parameters, 'weight')
    tmp_175 = __temp_78
    tmp_176 = tmp_4
    tmp_177 = tmp_4
    tmp_178 = tmp_4
    __temp_82 = tmp_4(tmp_4(tmp_159, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_179 = __temp_80
    tmp_180 = tmp_4
    tmp_181 = tmp_4
    tmp_182 = tmp_4
    tmp_183 = tmp_4
    tmp_184 = tmp_4
    tmp_185 = tmp_4
    __temp_85 = tmp_4(tmp_4(tmp_4(tmp_159, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_186 = __temp_82
    tmp_187 = tmp_4
    tmp_188 = tmp_4
    tmp_189 = tmp_4
    __temp_87 = tmp_4(tmp_4(tmp_182, 'down_proj')._parameters, 'weight')
    tmp_190 = __temp_85
    tmp_191 = tmp_4
    tmp_192 = tmp_4
    tmp_193 = tmp_4
    tmp_194 = tmp_4
    tmp_195 = tmp_4
    tmp_196 = tmp_4
    __temp_90 = tmp_4(tmp_4(tmp_4(tmp_14, '5')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_197 = __temp_87
    tmp_198 = tmp_4
    tmp_199 = tmp_4
    tmp_200 = tmp_4
    tmp_201 = tmp_4
    tmp_202 = tmp_4
    tmp_203 = tmp_4
    __temp_93 = tmp_4(tmp_4(tmp_4(tmp_193, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_204 = __temp_90
    __temp_94 = tmp_4(tmp_203, 'weight')
    tmp_205 = __temp_93
    tmp_206 = tmp_4
    tmp_207 = tmp_4
    tmp_208 = tmp_4
    __temp_96 = tmp_4(tmp_4(tmp_200, 'o_proj')._parameters, 'weight')
    tmp_209 = __temp_94
    tmp_210 = tmp_4
    tmp_211 = tmp_4
    tmp_212 = tmp_4
    __temp_98 = tmp_4(tmp_4(tmp_193, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_213 = __temp_96
    tmp_214 = tmp_4
    tmp_215 = tmp_4
    tmp_216 = tmp_4
    tmp_217 = tmp_4
    tmp_218 = tmp_4
    tmp_219 = tmp_4
    __temp_101 = tmp_4(tmp_4(tmp_4(tmp_193, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_220 = __temp_98
    tmp_221 = tmp_4
    tmp_222 = tmp_4
    tmp_223 = tmp_4
    __temp_103 = tmp_4(tmp_4(tmp_216, 'down_proj')._parameters, 'weight')
    tmp_224 = __temp_101
    tmp_225 = tmp_4
    tmp_226 = tmp_4
    tmp_227 = tmp_4
    tmp_228 = tmp_4
    tmp_229 = tmp_4
    tmp_230 = tmp_4
    __temp_106 = tmp_4(tmp_4(tmp_4(tmp_14, '6')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_231 = __temp_103
    tmp_232 = tmp_4
    tmp_233 = tmp_4
    tmp_234 = tmp_4
    tmp_235 = tmp_4
    tmp_236 = tmp_4
    tmp_237 = tmp_4
    __temp_109 = tmp_4(tmp_4(tmp_4(tmp_227, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_238 = __temp_106
    __temp_110 = tmp_4(tmp_237, 'weight')
    tmp_239 = __temp_109
    tmp_240 = tmp_4
    tmp_241 = tmp_4
    tmp_242 = tmp_4
    __temp_112 = tmp_4(tmp_4(tmp_234, 'o_proj')._parameters, 'weight')
    tmp_243 = __temp_110
    tmp_244 = tmp_4
    tmp_245 = tmp_4
    tmp_246 = tmp_4
    __temp_114 = tmp_4(tmp_4(tmp_227, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_247 = __temp_112
    tmp_248 = tmp_4
    tmp_249 = tmp_4
    tmp_250 = tmp_4
    tmp_251 = tmp_4
    tmp_252 = tmp_4
    tmp_253 = tmp_4
    __temp_117 = tmp_4(tmp_4(tmp_4(tmp_227, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_254 = __temp_114
    tmp_255 = tmp_4
    tmp_256 = tmp_4
    tmp_257 = tmp_4
    __temp_119 = tmp_4(tmp_4(tmp_250, 'down_proj')._parameters, 'weight')
    tmp_258 = __temp_117
    tmp_259 = tmp_4
    tmp_260 = tmp_4
    tmp_261 = tmp_4
    tmp_262 = tmp_4
    tmp_263 = tmp_4
    tmp_264 = tmp_4
    __temp_122 = tmp_4(tmp_4(tmp_4(tmp_14, '7')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_265 = __temp_119
    tmp_266 = tmp_4
    tmp_267 = tmp_4
    tmp_268 = tmp_4
    tmp_269 = tmp_4
    tmp_270 = tmp_4
    tmp_271 = tmp_4
    __temp_125 = tmp_4(tmp_4(tmp_4(tmp_261, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_272 = __temp_122
    __temp_126 = tmp_4(tmp_271, 'weight')
    tmp_273 = __temp_125
    tmp_274 = tmp_4
    tmp_275 = tmp_4
    tmp_276 = tmp_4
    __temp_128 = tmp_4(tmp_4(tmp_268, 'o_proj')._parameters, 'weight')
    tmp_277 = __temp_126
    tmp_278 = tmp_4
    tmp_279 = tmp_4
    tmp_280 = tmp_4
    __temp_130 = tmp_4(tmp_4(tmp_261, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_281 = __temp_128
    tmp_282 = tmp_4
    tmp_283 = tmp_4
    tmp_284 = tmp_4
    tmp_285 = tmp_4
    tmp_286 = tmp_4
    tmp_287 = tmp_4
    __temp_133 = tmp_4(tmp_4(tmp_4(tmp_261, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_288 = __temp_130
    tmp_289 = tmp_4
    tmp_290 = tmp_4
    tmp_291 = tmp_4
    __temp_135 = tmp_4(tmp_4(tmp_284, 'down_proj')._parameters, 'weight')
    tmp_292 = __temp_133
    tmp_293 = tmp_4
    tmp_294 = tmp_4
    tmp_295 = tmp_4
    tmp_296 = tmp_4
    tmp_297 = tmp_4
    tmp_298 = tmp_4
    __temp_138 = tmp_4(tmp_4(tmp_4(tmp_14, '8')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_299 = __temp_135
    tmp_300 = tmp_4
    tmp_301 = tmp_4
    tmp_302 = tmp_4
    tmp_303 = tmp_4
    tmp_304 = tmp_4
    tmp_305 = tmp_4
    __temp_141 = tmp_4(tmp_4(tmp_4(tmp_295, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_306 = __temp_138
    __temp_142 = tmp_4(tmp_305, 'weight')
    tmp_307 = __temp_141
    tmp_308 = tmp_4
    tmp_309 = tmp_4
    tmp_310 = tmp_4
    __temp_144 = tmp_4(tmp_4(tmp_302, 'o_proj')._parameters, 'weight')
    tmp_311 = __temp_142
    tmp_312 = tmp_4
    tmp_313 = tmp_4
    tmp_314 = tmp_4
    __temp_146 = tmp_4(tmp_4(tmp_295, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_315 = __temp_144
    tmp_316 = tmp_4
    tmp_317 = tmp_4
    tmp_318 = tmp_4
    tmp_319 = tmp_4
    tmp_320 = tmp_4
    tmp_321 = tmp_4
    __temp_149 = tmp_4(tmp_4(tmp_4(tmp_295, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_322 = __temp_146
    tmp_323 = tmp_4
    tmp_324 = tmp_4
    tmp_325 = tmp_4
    __temp_151 = tmp_4(tmp_4(tmp_318, 'down_proj')._parameters, 'weight')
    tmp_326 = __temp_149
    tmp_327 = tmp_4
    tmp_328 = tmp_4
    tmp_329 = tmp_4
    tmp_330 = tmp_4
    tmp_331 = tmp_4
    tmp_332 = tmp_4
    __temp_154 = tmp_4(tmp_4(tmp_4(tmp_14, '9')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_333 = __temp_151
    tmp_334 = tmp_4
    tmp_335 = tmp_4
    tmp_336 = tmp_4
    tmp_337 = tmp_4
    tmp_338 = tmp_4
    tmp_339 = tmp_4
    __temp_157 = tmp_4(tmp_4(tmp_4(tmp_329, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_340 = __temp_154
    __temp_158 = tmp_4(tmp_339, 'weight')
    tmp_341 = __temp_157
    tmp_342 = tmp_4
    tmp_343 = tmp_4
    tmp_344 = tmp_4
    __temp_160 = tmp_4(tmp_4(tmp_336, 'o_proj')._parameters, 'weight')
    tmp_345 = __temp_158
    tmp_346 = tmp_4
    tmp_347 = tmp_4
    tmp_348 = tmp_4
    __temp_162 = tmp_4(tmp_4(tmp_329, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_349 = __temp_160
    tmp_350 = tmp_4
    tmp_351 = tmp_4
    tmp_352 = tmp_4
    tmp_353 = tmp_4
    tmp_354 = tmp_4
    tmp_355 = tmp_4
    __temp_165 = tmp_4(tmp_4(tmp_4(tmp_329, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_356 = __temp_162
    tmp_357 = tmp_4
    tmp_358 = tmp_4
    tmp_359 = tmp_4
    __temp_167 = tmp_4(tmp_4(tmp_352, 'down_proj')._parameters, 'weight')
    tmp_360 = __temp_165
    tmp_361 = tmp_4
    tmp_362 = tmp_4
    tmp_363 = tmp_4
    tmp_364 = tmp_4
    tmp_365 = tmp_4
    tmp_366 = tmp_4
    __temp_170 = tmp_4(tmp_4(tmp_4(tmp_14, '10')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_367 = __temp_167
    tmp_368 = tmp_4
    tmp_369 = tmp_4
    tmp_370 = tmp_4
    tmp_371 = tmp_4
    tmp_372 = tmp_4
    tmp_373 = tmp_4
    __temp_173 = tmp_4(tmp_4(tmp_4(tmp_363, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_374 = __temp_170
    __temp_174 = tmp_4(tmp_373, 'weight')
    tmp_375 = __temp_173
    tmp_376 = tmp_4
    tmp_377 = tmp_4
    tmp_378 = tmp_4
    __temp_176 = tmp_4(tmp_4(tmp_370, 'o_proj')._parameters, 'weight')
    tmp_379 = __temp_174
    tmp_380 = tmp_4
    tmp_381 = tmp_4
    tmp_382 = tmp_4
    __temp_178 = tmp_4(tmp_4(tmp_363, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_383 = __temp_176
    tmp_384 = tmp_4
    tmp_385 = tmp_4
    tmp_386 = tmp_4
    tmp_387 = tmp_4
    tmp_388 = tmp_4
    tmp_389 = tmp_4
    __temp_181 = tmp_4(tmp_4(tmp_4(tmp_363, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_390 = __temp_178
    tmp_391 = tmp_4
    tmp_392 = tmp_4
    tmp_393 = tmp_4
    __temp_183 = tmp_4(tmp_4(tmp_386, 'down_proj')._parameters, 'weight')
    tmp_394 = __temp_181
    tmp_395 = tmp_4
    tmp_396 = tmp_4
    tmp_397 = tmp_4
    tmp_398 = tmp_4
    tmp_399 = tmp_4
    tmp_400 = tmp_4
    __temp_186 = tmp_4(tmp_4(tmp_4(tmp_14, '11')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_401 = __temp_183
    tmp_402 = tmp_4
    tmp_403 = tmp_4
    tmp_404 = tmp_4
    tmp_405 = tmp_4
    tmp_406 = tmp_4
    tmp_407 = tmp_4
    __temp_189 = tmp_4(tmp_4(tmp_4(tmp_397, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_408 = __temp_186
    __temp_190 = tmp_4(tmp_407, 'weight')
    tmp_409 = __temp_189
    tmp_410 = tmp_4
    tmp_411 = tmp_4
    tmp_412 = tmp_4
    __temp_192 = tmp_4(tmp_4(tmp_404, 'o_proj')._parameters, 'weight')
    tmp_413 = __temp_190
    tmp_414 = tmp_4
    tmp_415 = tmp_4
    tmp_416 = tmp_4
    __temp_194 = tmp_4(tmp_4(tmp_397, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_417 = __temp_192
    tmp_418 = tmp_4
    tmp_419 = tmp_4
    tmp_420 = tmp_4
    tmp_421 = tmp_4
    tmp_422 = tmp_4
    tmp_423 = tmp_4
    __temp_197 = tmp_4(tmp_4(tmp_4(tmp_397, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_424 = __temp_194
    tmp_425 = tmp_4
    tmp_426 = tmp_4
    tmp_427 = tmp_4
    __temp_199 = tmp_4(tmp_4(tmp_420, 'down_proj')._parameters, 'weight')
    tmp_428 = __temp_197
    tmp_429 = tmp_4
    tmp_430 = tmp_4
    tmp_431 = tmp_4
    tmp_432 = tmp_4
    tmp_433 = tmp_4
    tmp_434 = tmp_4
    __temp_202 = tmp_4(tmp_4(tmp_4(tmp_14, '12')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_435 = __temp_199
    tmp_436 = tmp_4
    tmp_437 = tmp_4
    tmp_438 = tmp_4
    tmp_439 = tmp_4
    tmp_440 = tmp_4
    tmp_441 = tmp_4
    __temp_205 = tmp_4(tmp_4(tmp_4(tmp_431, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_442 = __temp_202
    __temp_206 = tmp_4(tmp_441, 'weight')
    tmp_443 = __temp_205
    tmp_444 = tmp_4
    tmp_445 = tmp_4
    tmp_446 = tmp_4
    __temp_208 = tmp_4(tmp_4(tmp_438, 'o_proj')._parameters, 'weight')
    tmp_447 = __temp_206
    tmp_448 = tmp_4
    tmp_449 = tmp_4
    tmp_450 = tmp_4
    __temp_210 = tmp_4(tmp_4(tmp_431, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_451 = __temp_208
    tmp_452 = tmp_4
    tmp_453 = tmp_4
    tmp_454 = tmp_4
    tmp_455 = tmp_4
    tmp_456 = tmp_4
    tmp_457 = tmp_4
    __temp_213 = tmp_4(tmp_4(tmp_4(tmp_431, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_458 = __temp_210
    tmp_459 = tmp_4
    tmp_460 = tmp_4
    tmp_461 = tmp_4
    __temp_215 = tmp_4(tmp_4(tmp_454, 'down_proj')._parameters, 'weight')
    tmp_462 = __temp_213
    tmp_463 = tmp_4
    tmp_464 = tmp_4
    tmp_465 = tmp_4
    tmp_466 = tmp_4
    tmp_467 = tmp_4
    tmp_468 = tmp_4
    __temp_218 = tmp_4(tmp_4(tmp_4(tmp_14, '13')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_469 = __temp_215
    tmp_470 = tmp_4
    tmp_471 = tmp_4
    tmp_472 = tmp_4
    tmp_473 = tmp_4
    tmp_474 = tmp_4
    tmp_475 = tmp_4
    __temp_221 = tmp_4(tmp_4(tmp_4(tmp_465, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_476 = __temp_218
    __temp_222 = tmp_4(tmp_475, 'weight')
    tmp_477 = __temp_221
    tmp_478 = tmp_4
    tmp_479 = tmp_4
    tmp_480 = tmp_4
    __temp_224 = tmp_4(tmp_4(tmp_472, 'o_proj')._parameters, 'weight')
    tmp_481 = __temp_222
    tmp_482 = tmp_4
    tmp_483 = tmp_4
    tmp_484 = tmp_4
    __temp_226 = tmp_4(tmp_4(tmp_465, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_485 = __temp_224
    tmp_486 = tmp_4
    tmp_487 = tmp_4
    tmp_488 = tmp_4
    tmp_489 = tmp_4
    tmp_490 = tmp_4
    tmp_491 = tmp_4
    __temp_229 = tmp_4(tmp_4(tmp_4(tmp_465, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_492 = __temp_226
    tmp_493 = tmp_4
    tmp_494 = tmp_4
    tmp_495 = tmp_4
    __temp_231 = tmp_4(tmp_4(tmp_488, 'down_proj')._parameters, 'weight')
    tmp_496 = __temp_229
    tmp_497 = tmp_4
    tmp_498 = tmp_4
    tmp_499 = tmp_4
    tmp_500 = tmp_4
    tmp_501 = tmp_4
    tmp_502 = tmp_4
    __temp_234 = tmp_4(tmp_4(tmp_4(tmp_14, '14')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_503 = __temp_231
    tmp_504 = tmp_4
    tmp_505 = tmp_4
    tmp_506 = tmp_4
    tmp_507 = tmp_4
    tmp_508 = tmp_4
    tmp_509 = tmp_4
    __temp_237 = tmp_4(tmp_4(tmp_4(tmp_499, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_510 = __temp_234
    __temp_238 = tmp_4(tmp_509, 'weight')
    tmp_511 = __temp_237
    tmp_512 = tmp_4
    tmp_513 = tmp_4
    tmp_514 = tmp_4
    __temp_240 = tmp_4(tmp_4(tmp_506, 'o_proj')._parameters, 'weight')
    tmp_515 = __temp_238
    tmp_516 = tmp_4
    tmp_517 = tmp_4
    tmp_518 = tmp_4
    __temp_242 = tmp_4(tmp_4(tmp_499, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_519 = __temp_240
    tmp_520 = tmp_4
    tmp_521 = tmp_4
    tmp_522 = tmp_4
    tmp_523 = tmp_4
    tmp_524 = tmp_4
    tmp_525 = tmp_4
    __temp_245 = tmp_4(tmp_4(tmp_4(tmp_499, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_526 = __temp_242
    tmp_527 = tmp_4
    tmp_528 = tmp_4
    tmp_529 = tmp_4
    __temp_247 = tmp_4(tmp_4(tmp_522, 'down_proj')._parameters, 'weight')
    tmp_530 = __temp_245
    tmp_531 = tmp_4
    tmp_532 = tmp_4
    tmp_533 = tmp_4
    tmp_534 = tmp_4
    tmp_535 = tmp_4
    tmp_536 = tmp_4
    __temp_250 = tmp_4(tmp_4(tmp_4(tmp_14, '15')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_537 = __temp_247
    tmp_538 = tmp_4
    tmp_539 = tmp_4
    tmp_540 = tmp_4
    tmp_541 = tmp_4
    tmp_542 = tmp_4
    tmp_543 = tmp_4
    __temp_253 = tmp_4(tmp_4(tmp_4(tmp_533, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_544 = __temp_250
    __temp_254 = tmp_4(tmp_543, 'weight')
    tmp_545 = __temp_253
    tmp_546 = tmp_4
    tmp_547 = tmp_4
    tmp_548 = tmp_4
    __temp_256 = tmp_4(tmp_4(tmp_540, 'o_proj')._parameters, 'weight')
    tmp_549 = __temp_254
    tmp_550 = tmp_4
    tmp_551 = tmp_4
    tmp_552 = tmp_4
    __temp_258 = tmp_4(tmp_4(tmp_533, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_553 = __temp_256
    tmp_554 = tmp_4
    tmp_555 = tmp_4
    tmp_556 = tmp_4
    tmp_557 = tmp_4
    tmp_558 = tmp_4
    tmp_559 = tmp_4
    __temp_261 = tmp_4(tmp_4(tmp_4(tmp_533, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_560 = __temp_258
    tmp_561 = tmp_4
    tmp_562 = tmp_4
    tmp_563 = tmp_4
    __temp_263 = tmp_4(tmp_4(tmp_556, 'down_proj')._parameters, 'weight')
    tmp_564 = __temp_261
    tmp_565 = tmp_4
    tmp_566 = tmp_4
    tmp_567 = tmp_4
    tmp_568 = tmp_4
    tmp_569 = tmp_4
    tmp_570 = tmp_4
    __temp_266 = tmp_4(tmp_4(tmp_4(tmp_14, '16')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_571 = __temp_263
    tmp_572 = tmp_4
    tmp_573 = tmp_4
    tmp_574 = tmp_4
    tmp_575 = tmp_4
    tmp_576 = tmp_4
    tmp_577 = tmp_4
    __temp_269 = tmp_4(tmp_4(tmp_4(tmp_567, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_578 = __temp_266
    __temp_270 = tmp_4(tmp_577, 'weight')
    tmp_579 = __temp_269
    tmp_580 = tmp_4
    tmp_581 = tmp_4
    tmp_582 = tmp_4
    __temp_272 = tmp_4(tmp_4(tmp_574, 'o_proj')._parameters, 'weight')
    tmp_583 = __temp_270
    tmp_584 = tmp_4
    tmp_585 = tmp_4
    tmp_586 = tmp_4
    __temp_274 = tmp_4(tmp_4(tmp_567, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_587 = __temp_272
    tmp_588 = tmp_4
    tmp_589 = tmp_4
    tmp_590 = tmp_4
    tmp_591 = tmp_4
    tmp_592 = tmp_4
    tmp_593 = tmp_4
    __temp_277 = tmp_4(tmp_4(tmp_4(tmp_567, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_594 = __temp_274
    tmp_595 = tmp_4
    tmp_596 = tmp_4
    tmp_597 = tmp_4
    __temp_279 = tmp_4(tmp_4(tmp_590, 'down_proj')._parameters, 'weight')
    tmp_598 = __temp_277
    tmp_599 = tmp_4
    tmp_600 = tmp_4
    tmp_601 = tmp_4
    tmp_602 = tmp_4
    tmp_603 = tmp_4
    tmp_604 = tmp_4
    __temp_282 = tmp_4(tmp_4(tmp_4(tmp_14, '17')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_605 = __temp_279
    tmp_606 = tmp_4
    tmp_607 = tmp_4
    tmp_608 = tmp_4
    tmp_609 = tmp_4
    tmp_610 = tmp_4
    tmp_611 = tmp_4
    __temp_285 = tmp_4(tmp_4(tmp_4(tmp_601, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_612 = __temp_282
    __temp_286 = tmp_4(tmp_611, 'weight')
    tmp_613 = __temp_285
    tmp_614 = tmp_4
    tmp_615 = tmp_4
    tmp_616 = tmp_4
    __temp_288 = tmp_4(tmp_4(tmp_608, 'o_proj')._parameters, 'weight')
    tmp_617 = __temp_286
    tmp_618 = tmp_4
    tmp_619 = tmp_4
    tmp_620 = tmp_4
    __temp_290 = tmp_4(tmp_4(tmp_601, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_621 = __temp_288
    tmp_622 = tmp_4
    tmp_623 = tmp_4
    tmp_624 = tmp_4
    tmp_625 = tmp_4
    tmp_626 = tmp_4
    tmp_627 = tmp_4
    __temp_293 = tmp_4(tmp_4(tmp_4(tmp_601, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_628 = __temp_290
    tmp_629 = tmp_4
    tmp_630 = tmp_4
    tmp_631 = tmp_4
    __temp_295 = tmp_4(tmp_4(tmp_624, 'down_proj')._parameters, 'weight')
    tmp_632 = __temp_293
    tmp_633 = tmp_4
    tmp_634 = tmp_4
    tmp_635 = tmp_4
    tmp_636 = tmp_4
    tmp_637 = tmp_4
    tmp_638 = tmp_4
    __temp_298 = tmp_4(tmp_4(tmp_4(tmp_14, '18')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_639 = __temp_295
    tmp_640 = tmp_4
    tmp_641 = tmp_4
    tmp_642 = tmp_4
    tmp_643 = tmp_4
    tmp_644 = tmp_4
    tmp_645 = tmp_4
    __temp_301 = tmp_4(tmp_4(tmp_4(tmp_635, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_646 = __temp_298
    __temp_302 = tmp_4(tmp_645, 'weight')
    tmp_647 = __temp_301
    tmp_648 = tmp_4
    tmp_649 = tmp_4
    tmp_650 = tmp_4
    __temp_304 = tmp_4(tmp_4(tmp_642, 'o_proj')._parameters, 'weight')
    tmp_651 = __temp_302
    tmp_652 = tmp_4
    tmp_653 = tmp_4
    tmp_654 = tmp_4
    __temp_306 = tmp_4(tmp_4(tmp_635, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_655 = __temp_304
    tmp_656 = tmp_4
    tmp_657 = tmp_4
    tmp_658 = tmp_4
    tmp_659 = tmp_4
    tmp_660 = tmp_4
    tmp_661 = tmp_4
    __temp_309 = tmp_4(tmp_4(tmp_4(tmp_635, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_662 = __temp_306
    tmp_663 = tmp_4
    tmp_664 = tmp_4
    tmp_665 = tmp_4
    __temp_311 = tmp_4(tmp_4(tmp_658, 'down_proj')._parameters, 'weight')
    tmp_666 = __temp_309
    tmp_667 = tmp_4
    tmp_668 = tmp_4
    tmp_669 = tmp_4
    tmp_670 = tmp_4
    tmp_671 = tmp_4
    tmp_672 = tmp_4
    __temp_314 = tmp_4(tmp_4(tmp_4(tmp_14, '19')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_673 = __temp_311
    tmp_674 = tmp_4
    tmp_675 = tmp_4
    tmp_676 = tmp_4
    tmp_677 = tmp_4
    tmp_678 = tmp_4
    tmp_679 = tmp_4
    __temp_317 = tmp_4(tmp_4(tmp_4(tmp_669, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_680 = __temp_314
    __temp_318 = tmp_4(tmp_679, 'weight')
    tmp_681 = __temp_317
    tmp_682 = tmp_4
    tmp_683 = tmp_4
    tmp_684 = tmp_4
    __temp_320 = tmp_4(tmp_4(tmp_676, 'o_proj')._parameters, 'weight')
    tmp_685 = __temp_318
    tmp_686 = tmp_4
    tmp_687 = tmp_4
    tmp_688 = tmp_4
    __temp_322 = tmp_4(tmp_4(tmp_669, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_689 = __temp_320
    tmp_690 = tmp_4
    tmp_691 = tmp_4
    tmp_692 = tmp_4
    tmp_693 = tmp_4
    tmp_694 = tmp_4
    tmp_695 = tmp_4
    __temp_325 = tmp_4(tmp_4(tmp_4(tmp_669, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_696 = __temp_322
    tmp_697 = tmp_4
    tmp_698 = tmp_4
    tmp_699 = tmp_4
    __temp_327 = tmp_4(tmp_4(tmp_692, 'down_proj')._parameters, 'weight')
    tmp_700 = __temp_325
    tmp_701 = tmp_4
    tmp_702 = tmp_4
    tmp_703 = tmp_4
    tmp_704 = tmp_4
    tmp_705 = tmp_4
    tmp_706 = tmp_4
    __temp_330 = tmp_4(tmp_4(tmp_4(tmp_14, '20')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_707 = __temp_327
    tmp_708 = tmp_4
    tmp_709 = tmp_4
    tmp_710 = tmp_4
    tmp_711 = tmp_4
    tmp_712 = tmp_4
    tmp_713 = tmp_4
    __temp_333 = tmp_4(tmp_4(tmp_4(tmp_703, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_714 = __temp_330
    __temp_334 = tmp_4(tmp_713, 'weight')
    tmp_715 = __temp_333
    tmp_716 = tmp_4
    tmp_717 = tmp_4
    tmp_718 = tmp_4
    __temp_336 = tmp_4(tmp_4(tmp_710, 'o_proj')._parameters, 'weight')
    tmp_719 = __temp_334
    tmp_720 = tmp_4
    tmp_721 = tmp_4
    tmp_722 = tmp_4
    __temp_338 = tmp_4(tmp_4(tmp_703, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_723 = __temp_336
    tmp_724 = tmp_4
    tmp_725 = tmp_4
    tmp_726 = tmp_4
    tmp_727 = tmp_4
    tmp_728 = tmp_4
    tmp_729 = tmp_4
    __temp_341 = tmp_4(tmp_4(tmp_4(tmp_703, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_730 = __temp_338
    tmp_731 = tmp_4
    tmp_732 = tmp_4
    tmp_733 = tmp_4
    __temp_343 = tmp_4(tmp_4(tmp_726, 'down_proj')._parameters, 'weight')
    tmp_734 = __temp_341
    tmp_735 = tmp_4
    tmp_736 = tmp_4
    tmp_737 = tmp_4
    tmp_738 = tmp_4
    tmp_739 = tmp_4
    tmp_740 = tmp_4
    __temp_346 = tmp_4(tmp_4(tmp_4(tmp_14, '21')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_741 = __temp_343
    tmp_742 = tmp_4
    tmp_743 = tmp_4
    tmp_744 = tmp_4
    tmp_745 = tmp_4
    tmp_746 = tmp_4
    tmp_747 = tmp_4
    __temp_349 = tmp_4(tmp_4(tmp_4(tmp_737, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_748 = __temp_346
    __temp_350 = tmp_4(tmp_747, 'weight')
    tmp_749 = __temp_349
    tmp_750 = tmp_4
    tmp_751 = tmp_4
    tmp_752 = tmp_4
    __temp_352 = tmp_4(tmp_4(tmp_744, 'o_proj')._parameters, 'weight')
    tmp_753 = __temp_350
    tmp_754 = tmp_4
    tmp_755 = tmp_4
    tmp_756 = tmp_4
    __temp_354 = tmp_4(tmp_4(tmp_737, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_757 = __temp_352
    tmp_758 = tmp_4
    tmp_759 = tmp_4
    tmp_760 = tmp_4
    tmp_761 = tmp_4
    tmp_762 = tmp_4
    tmp_763 = tmp_4
    __temp_357 = tmp_4(tmp_4(tmp_4(tmp_737, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_764 = __temp_354
    tmp_765 = tmp_4
    tmp_766 = tmp_4
    tmp_767 = tmp_4
    __temp_359 = tmp_4(tmp_4(tmp_760, 'down_proj')._parameters, 'weight')
    tmp_768 = __temp_357
    tmp_769 = tmp_4
    tmp_770 = tmp_4
    tmp_771 = tmp_4
    tmp_772 = tmp_4
    tmp_773 = tmp_4
    tmp_774 = tmp_4
    __temp_362 = tmp_4(tmp_4(tmp_4(tmp_14, '22')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_775 = __temp_359
    tmp_776 = tmp_4
    tmp_777 = tmp_4
    tmp_778 = tmp_4
    tmp_779 = tmp_4
    tmp_780 = tmp_4
    tmp_781 = tmp_4
    __temp_365 = tmp_4(tmp_4(tmp_4(tmp_771, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_782 = __temp_362
    __temp_366 = tmp_4(tmp_781, 'weight')
    tmp_783 = __temp_365
    tmp_784 = tmp_4
    tmp_785 = tmp_4
    tmp_786 = tmp_4
    __temp_368 = tmp_4(tmp_4(tmp_778, 'o_proj')._parameters, 'weight')
    tmp_787 = __temp_366
    tmp_788 = tmp_4
    tmp_789 = tmp_4
    tmp_790 = tmp_4
    __temp_370 = tmp_4(tmp_4(tmp_771, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_791 = __temp_368
    tmp_792 = tmp_4
    tmp_793 = tmp_4
    tmp_794 = tmp_4
    tmp_795 = tmp_4
    tmp_796 = tmp_4
    tmp_797 = tmp_4
    __temp_373 = tmp_4(tmp_4(tmp_4(tmp_771, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_798 = __temp_370
    tmp_799 = tmp_4
    tmp_800 = tmp_4
    tmp_801 = tmp_4
    __temp_375 = tmp_4(tmp_4(tmp_794, 'down_proj')._parameters, 'weight')
    tmp_802 = __temp_373
    tmp_803 = tmp_4
    tmp_804 = tmp_4
    tmp_805 = tmp_4
    tmp_806 = tmp_4
    tmp_807 = tmp_4
    tmp_808 = tmp_4
    __temp_378 = tmp_4(tmp_4(tmp_4(tmp_14, '23')._modules, 'input_layernorm').
        _parameters, 'weight')
    tmp_809 = __temp_375
    tmp_810 = tmp_4
    tmp_811 = tmp_4
    tmp_812 = tmp_4
    tmp_813 = tmp_4
    tmp_814 = tmp_4
    tmp_815 = tmp_4
    __temp_381 = tmp_4(tmp_4(tmp_4(tmp_805, 'self_attn')._modules, 'qkv_proj').
        _parameters, 'bias')
    tmp_816 = __temp_378
    __temp_382 = tmp_4(tmp_815, 'weight')
    tmp_817 = __temp_381
    tmp_818 = tmp_4
    tmp_819 = tmp_4
    tmp_820 = tmp_4
    __temp_384 = tmp_4(tmp_4(tmp_812, 'o_proj')._parameters, 'weight')
    tmp_821 = __temp_382
    tmp_822 = tmp_4
    tmp_823 = tmp_4
    tmp_824 = tmp_4
    __temp_386 = tmp_4(tmp_4(tmp_805, 'post_attention_layernorm')._parameters,
        'weight')
    tmp_825 = __temp_384
    tmp_826 = tmp_4
    tmp_827 = tmp_4
    tmp_828 = tmp_4
    tmp_829 = tmp_4
    tmp_830 = tmp_4
    tmp_831 = tmp_4
    __temp_389 = tmp_4(tmp_4(tmp_4(tmp_805, 'mlp')._modules, 'gate_up_proj').
        _parameters, 'weight')
    tmp_832 = __temp_386
    tmp_833 = tmp_4
    tmp_834 = tmp_4
    tmp_835 = tmp_4
    __temp_391 = tmp_4(tmp_4(tmp_828, 'down_proj')._parameters, 'weight')
    tmp_836 = __temp_389
    tmp_837 = tmp_4
    tmp_838 = tmp_4
    tmp_839 = tmp_4
    tmp_840 = __temp_391
    __temp_395, = __compiled_fn_1(__import_torch_dot__dynamo_dot_utils.
        call_size(input_ids, 0), tmp_2, __temp_3, __temp_7, __temp_10,
        __temp_11, tmp_1(positions, 0), tmp_30, __temp_14, __temp_16, __temp_18,
        __temp_21, __temp_23, __temp_26, __temp_29, __temp_30, __temp_32,
        __temp_34, __temp_37, __temp_39, __temp_42, __temp_45, __temp_46,
        __temp_48, __temp_50, __temp_53, __temp_55, __temp_58, __temp_61,
        __temp_62, __temp_64, __temp_66, __temp_69, __temp_71, __temp_74,
        __temp_77, __temp_78, __temp_80, __temp_82, __temp_85, __temp_87,
        __temp_90, __temp_93, __temp_94, __temp_96, __temp_98, __temp_101,
        __temp_103, __temp_106, __temp_109, __temp_110, __temp_112, __temp_114,
        __temp_117, __temp_119, __temp_122, __temp_125, __temp_126, __temp_128,
        __temp_130, __temp_133, __temp_135, __temp_138, __temp_141, __temp_142,
        __temp_144, __temp_146, __temp_149, __temp_151, __temp_154, __temp_157,
        __temp_158, __temp_160, __temp_162, __temp_165, __temp_167, __temp_170,
        __temp_173, __temp_174, __temp_176, __temp_178, __temp_181, __temp_183,
        __temp_186, __temp_189, __temp_190, __temp_192, __temp_194, __temp_197,
        __temp_199, __temp_202, __temp_205, __temp_206, __temp_208, __temp_210,
        __temp_213, __temp_215, __temp_218, __temp_221, __temp_222, __temp_224,
        __temp_226, __temp_229, __temp_231, __temp_234, __temp_237, __temp_238,
        __temp_240, __temp_242, __temp_245, __temp_247, __temp_250, __temp_253,
        __temp_254, __temp_256, __temp_258, __temp_261, __temp_263, __temp_266,
        __temp_269, __temp_270, __temp_272, __temp_274, __temp_277, __temp_279,
        __temp_282, __temp_285, __temp_286, __temp_288, __temp_290, __temp_293,
        __temp_295, __temp_298, __temp_301, __temp_302, __temp_304, __temp_306,
        __temp_309, __temp_311, __temp_314, __temp_317, __temp_318, __temp_320,
        __temp_322, __temp_325, __temp_327, __temp_330, __temp_333, __temp_334,
        __temp_336, __temp_338, __temp_341, __temp_343, __temp_346, __temp_349,
        __temp_350, __temp_352, __temp_354, __temp_357, __temp_359, __temp_362,
        __temp_365, __temp_366, __temp_368, __temp_370, __temp_373, __temp_375,
        __temp_378, __temp_381, __temp_382, __temp_384, __temp_386, __temp_389,
        __temp_391, tmp_4(tmp_4(tmp_7, 'norm')._parameters, 'weight'))
    return __temp_395
