#!/usr/bin/env python3
"""
七牛云 ASR WebSocket 协议测试工具
基于官方文档实现，用于验证服务端 ASR 实现的正确性
"""

import asyncio
import gzip
import json
import time
import uuid
import websockets
import base64
import wave
import struct
from pathlib import Path

# -------------------- 协议相关常量和函数 --------------------

PROTOCOL_VERSION = 0b0001

# Message Types
FULL_CLIENT_REQUEST = 0b0001
AUDIO_ONLY_REQUEST = 0b0010
FULL_SERVER_RESPONSE = 0b1001
SERVER_ACK = 0b1011
SERVER_ERROR_RESPONSE = 0b1111

# Message Type Specific Flags
NO_SEQUENCE = 0b0000
POS_SEQUENCE = 0b0001
NEG_SEQUENCE = 0b0010
NEG_WITH_SEQUENCE = 0b0011

# 序列化和压缩方式
NO_SERIALIZATION = 0b0000
JSON_SERIALIZATION = 0b0001
NO_COMPRESSION = 0b0000
GZIP_COMPRESSION = 0b0001

def generate_header(message_type=FULL_CLIENT_REQUEST,
                    message_type_specific_flags=NO_SEQUENCE,
                    serial_method=JSON_SERIALIZATION,
                    compression_type=GZIP_COMPRESSION,
                    reserved_data=0x00):
    header = bytearray()
    header_size = 1
    header.append((PROTOCOL_VERSION << 4) | header_size)
    header.append((message_type << 4) | message_type_specific_flags)
    header.append((serial_method << 4) | compression_type)
    header.append(reserved_data)
    return header

def generate_before_payload(sequence: int):
    before_payload = bytearray()
    before_payload.extend(sequence.to_bytes(4, 'big', signed=True))
    return before_payload

def parse_response(res):
    """
    解析服务器响应，兼容多种格式
    """
    if not isinstance(res, bytes):
        return {'payload_msg': res}
    
    if len(res) < 4:
        return {'payload_msg': 'Invalid response: too short'}
    
    header_size = res[0] & 0x0f
    message_type = res[1] >> 4
    message_type_specific_flags = res[1] & 0x0f
    serialization_method = res[2] >> 4
    message_compression = res[2] & 0x0f
    
    payload = res[header_size * 4:]
    result = {}
    
    if message_type_specific_flags & 0x01:
        if len(payload) >= 4:
            seq = int.from_bytes(payload[:4], "big", signed=True)
            result['payload_sequence'] = seq
            payload = payload[4:]
    
    result['is_last_package'] = bool(message_type_specific_flags & 0x02)
    
    if message_type == FULL_SERVER_RESPONSE:
        if len(payload) >= 4:
            payload_size = int.from_bytes(payload[:4], "big", signed=True)
            payload_msg = payload[4:4+payload_size]
        else:
            payload_msg = payload
    elif message_type == SERVER_ACK:
        if len(payload) >= 4:
            seq = int.from_bytes(payload[:4], "big", signed=True)
            result['seq'] = seq
            if len(payload) >= 8:
                payload_size = int.from_bytes(payload[4:8], "big", signed=False)
                payload_msg = payload[8:8+payload_size]
            else:
                payload_msg = payload[4:]
        else:
            payload_msg = b""
    elif message_type == SERVER_ERROR_RESPONSE:
        if len(payload) >= 8:
            code = int.from_bytes(payload[:4], "big", signed=False)
            result['code'] = code
            payload_size = int.from_bytes(payload[4:8], "big", signed=False)
            payload_msg = payload[8:8+payload_size]
        else:
            payload_msg = payload
    else:
        payload_msg = payload

    if message_compression == GZIP_COMPRESSION:
        try:
            payload_msg = gzip.decompress(payload_msg)
        except Exception as e:
            print(f"GZIP decompression failed: {e}")
            pass
    
    if serialization_method == JSON_SERIALIZATION:
        try:
            payload_text = payload_msg.decode("utf-8")
            payload_msg = json.loads(payload_text)
        except Exception as e:
            print(f"JSON deserialization failed: {e}")
            try:
                payload_msg = payload_msg.decode("utf-8", errors="ignore")
            except:
                pass
    else:
        try:
            payload_msg = payload_msg.decode("utf-8", errors="ignore")
        except:
            pass
    
    result['payload_msg'] = payload_msg
    return result

def generate_test_pcm_audio(duration_ms=1000, sample_rate=16000, frequency=440):
    """
    生成测试用的 PCM 音频数据
    """
    num_samples = int(sample_rate * duration_ms / 1000)
    audio_data = []
    
    for i in range(num_samples):
        # 生成正弦波
        sample = int(32767 * 0.5 * math.sin(2 * math.pi * frequency * i / sample_rate))
        # 16位有符号整数，小端序
        audio_data.extend(struct.pack('<h', sample))
    
    return bytes(audio_data)

# -------------------- ASR 测试客户端 --------------------

class AsrTestClient:
    def __init__(self, token, ws_url, seg_duration=300, sample_rate=16000, channels=1, bits=16, format="pcm", **kwargs):
        """
        :param token: 鉴权 token
        :param ws_url: ASR websocket 服务地址
        :param seg_duration: 分段时长，单位毫秒
        :param sample_rate: 采样率（Hz）
        :param channels: 通道数（一般单声道为 1）
        :param bits: 采样位数（16 表示 16 位）
        :param format: 音频格式，这里设为 "pcm"
        """
        self.token = token
        self.ws_url = ws_url
        self.seg_duration = seg_duration  # 毫秒
        self.sample_rate = sample_rate
        self.channels = channels
        self.bits = bits
        self.format = format
        self.uid = kwargs.get("uid", "test")
        self.codec = kwargs.get("codec", "raw")
        self.streaming = kwargs.get("streaming", True)

    def construct_request(self, reqid):
        req = {
            "user": {"uid": self.uid},
            "audio": {
                "format": self.format,
                "sample_rate": self.sample_rate,
                "bits": self.bits,
                "channel": self.channels,
                "codec": self.codec,
            },
            "request": {"model_name": "asr", "enable_punc": True}
        }
        return req

    async def test_with_audio_file(self, audio_file_path):
        """
        使用音频文件进行测试
        """
        if not Path(audio_file_path).exists():
            print(f"音频文件不存在: {audio_file_path}")
            return

        # 读取音频文件（假设是 WAV 格式）
        try:
            with wave.open(audio_file_path, 'rb') as wav_file:
                audio_data = wav_file.readframes(wav_file.getnframes())
        except Exception as e:
            print(f"读取音频文件失败: {e}")
            return

        await self._test_with_audio_data(audio_data)

    async def test_with_generated_audio(self):
        """
        使用生成的测试音频进行测试
        """
        import math
        
        # 生成 3 秒的测试音频
        duration_ms = 3000
        num_samples = int(self.sample_rate * duration_ms / 1000)
        audio_data = bytearray()
        
        for i in range(num_samples):
            # 生成正弦波（440Hz，A4音符）
            sample = int(16383 * math.sin(2 * math.pi * 440 * i / self.sample_rate))
            # 16位有符号整数，小端序
            audio_data.extend(struct.pack('<h', sample))
        
        await self._test_with_audio_data(bytes(audio_data))

    async def _test_with_audio_data(self, audio_data):
        """
        使用给定的音频数据进行测试
        """
        reqid = str(uuid.uuid4())
        seq = 1
        request_params = self.construct_request(reqid)
        payload_bytes = json.dumps(request_params).encode("utf-8")
        payload_bytes = gzip.compress(payload_bytes)

        # 构造初始配置信息请求
        full_client_request = bytearray(generate_header(message_type_specific_flags=POS_SEQUENCE))
        full_client_request.extend(generate_before_payload(sequence=seq))
        full_client_request.extend((len(payload_bytes)).to_bytes(4, "big"))
        full_client_request.extend(payload_bytes)
        
        headers = {"Authorization": "Bearer " + self.token}
        begin_time = time.time()
        print(f"开始时间：{begin_time}")

        try:
            async with websockets.connect(self.ws_url, additional_headers=headers, max_size=1000000000) as ws:
                print("✅ WebSocket 连接成功")
                
                # 发送配置信息
                await ws.send(full_client_request)
                try:
                    res = await asyncio.wait_for(ws.recv(), timeout=10.0)
                except asyncio.TimeoutError:
                    print(f"❌ {time.time() - begin_time:.3f}s 等待配置信息响应超时")
                    return
                
                result = parse_response(res)
                print(f"✅ {time.time() - begin_time:.3f}s 配置响应：", result)

                # 分段发送音频数据
                bytes_per_frame = self.channels * (self.bits // 8)
                frames_per_segment = int(self.sample_rate * self.seg_duration / 1000)
                bytes_per_segment = frames_per_segment * bytes_per_frame
                
                total_segments = len(audio_data) // bytes_per_segment
                if len(audio_data) % bytes_per_segment != 0:
                    total_segments += 1
                
                print(f"📤 开始发送音频数据，总共 {total_segments} 段，每段 {bytes_per_segment} 字节")
                
                for i in range(total_segments):
                    start_idx = i * bytes_per_segment
                    end_idx = min((i + 1) * bytes_per_segment, len(audio_data))
                    chunk = audio_data[start_idx:end_idx]
                    
                    seq += 1
                    compressed_chunk = gzip.compress(chunk)
                    audio_only_request = bytearray(
                        generate_header(message_type=AUDIO_ONLY_REQUEST,
                                      message_type_specific_flags=POS_SEQUENCE))
                    audio_only_request.extend(generate_before_payload(sequence=seq))
                    audio_only_request.extend((len(compressed_chunk)).to_bytes(4, "big"))
                    audio_only_request.extend(compressed_chunk)
                    
                    await ws.send(audio_only_request)
                    print(f"📤 发送音频段 {i+1}/{total_segments} (序列号: {seq}, 大小: {len(chunk)} 字节)")
                    
                    try:
                        res = await asyncio.wait_for(ws.recv(), timeout=5.0)
                        result = parse_response(res)
                        elapsed = time.time() - begin_time
                        print(f"📥 {elapsed:.3f}s 接收响应：", result)
                    except asyncio.TimeoutError:
                        print(f"⚠️  音频段 {i+1} 响应超时")
                    
                    await asyncio.sleep(self.seg_duration / 1000.0)
                
                print("✅ 音频发送完成，等待最终结果...")
                
                # 等待最终结果
                try:
                    for _ in range(5):  # 最多等待 5 个额外响应
                        res = await asyncio.wait_for(ws.recv(), timeout=3.0)
                        result = parse_response(res)
                        elapsed = time.time() - begin_time
                        print(f"📥 {elapsed:.3f}s 最终响应：", result)
                except asyncio.TimeoutError:
                    print("✅ 测试完成")
                
        except Exception as e:
            print("❌ 异常：", e)

    def run_with_file(self, audio_file_path):
        asyncio.run(self.test_with_audio_file(audio_file_path))

    def run_with_generated_audio(self):
        asyncio.run(self.test_with_generated_audio())

# -------------------- 入口 --------------------

if __name__ == '__main__':
    import sys
    import math
    
    # 配置参数
    token = "sk-xxx"  # 请替换为实际的 token
    ws_url = "ws://localhost:8888/v1/chat/stream"  # 测试本地服务
    # ws_url = "wss://openai.qiniu.com/v1/voice/asr"  # 七牛云官方服务
    
    seg_duration = 300  # 分段时长，单位毫秒
    
    client = AsrTestClient(
        token=token, 
        ws_url=ws_url, 
        seg_duration=seg_duration,
        format="pcm"
    )
    
    print("🎤 ASR WebSocket 协议测试工具")
    print(f"📡 连接地址: {ws_url}")
    print(f"⏱️  分段时长: {seg_duration}ms")
    print("🎵 使用生成的测试音频进行测试...")
    
    # 使用生成的音频进行测试
    client.run_with_generated_audio()