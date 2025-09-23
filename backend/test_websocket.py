#!/usr/bin/env python3
"""
CosTalk WebSocket 测试客户端
用于测试 WebSocket 端点的配置、文本和音频消息功能
"""

import asyncio
import websockets
import json
import time

class CosTalkWSClient:
    def __init__(self, url):
        self.url = url
        self.websocket = None
        self.seq = 0
    
    async def connect(self):
        """连接到 WebSocket 服务器"""
        try:
            self.websocket = await websockets.connect(self.url)
            print(f"✅ 连接成功: {self.url}")
            return True
        except Exception as e:
            print(f"❌ 连接失败: {e}")
            return False
    
    async def disconnect(self):
        """断开 WebSocket 连接"""
        if self.websocket:
            await self.websocket.close()
            print("🔌 连接已断开")
    
    async def send_message(self, msg_type, content):
        """发送消息到服务器"""
        if not self.websocket:
            print("❌ 未连接到服务器")
            return
        
        self.seq += 1
        message = {
            "type": msg_type,
            "seq": self.seq,
            "content": content,
            "timestamp": int(time.time() * 1000)
        }
        
        try:
            await self.websocket.send(json.dumps(message))
            print(f"📤 发送 {msg_type}: {json.dumps(content, ensure_ascii=False)}")
        except Exception as e:
            print(f"❌ 发送失败: {e}")
    
    async def receive_messages(self):
        """持续接收来自服务器的消息"""
        if not self.websocket:
            print("❌ 未连接到服务器")
            return
        
        try:
            async for message in self.websocket:
                try:
                    data = json.loads(message)
                    msg_type = data.get('type', 'unknown')
                    content = data.get('content', {})
                    timestamp = data.get('timestamp', 0)
                    
                    # 根据消息类型显示不同的图标
                    icons = {
                        'response': '💬',
                        'error': '❌',
                        'asr': '🎤',
                        'tts': '🔊',
                        'config': '⚙️'
                    }
                    icon = icons.get(msg_type, '📨')
                    
                    print(f"{icon} 收到 {msg_type}: {json.dumps(content, ensure_ascii=False, indent=2)}")
                    
                except json.JSONDecodeError:
                    print(f"📨 收到原始消息: {message}")
                    
        except websockets.exceptions.ConnectionClosed:
            print("🔌 服务器连接已关闭")
        except Exception as e:
            print(f"❌ 接收消息错误: {e}")
    
    async def send_config(self):
        """发送配置消息"""
        config = {
            "llmProvider": "qiniu",
            "asrProvider": "qiniu", 
            "ttsProvider": "qiniu",
            "voice": "qiniu_zh_female_wwxkjx",
            "speed": 1.0,
            "role": "你是一个友好的AI助手，请用简洁明了的方式回答用户问题。"
        }
        await self.send_message("config", config)
    
    async def send_text(self, text):
        """发送文本消息"""
        content = {
            "content": text,
            "role": "user"
        }
        await self.send_message("text", content)
    
    async def send_audio(self, audio_data="test_audio_data"):
        """发送音频消息（模拟）"""
        await self.send_message("audio", audio_data)

async def main():
    """主测试函数"""
    url = "ws://localhost:8888/v1/chat/stream"
    client = CosTalkWSClient(url)
    
    print("🚀 CosTalk WebSocket 测试客户端")
    print("=" * 50)
    
    # 连接到服务器
    if not await client.connect():
        return
    
    # 启动消息接收任务
    receive_task = asyncio.create_task(client.receive_messages())
    
    try:
        # 等待服务器的欢迎消息
        await asyncio.sleep(1)
        
        print("\n📋 测试步骤:")
        
        # 1. 发送配置
        print("\n1️⃣  发送配置消息...")
        await client.send_config()
        await asyncio.sleep(2)
        
        # 2. 发送文本消息
        print("\n2️⃣  发送文本消息...")
        await client.send_text("你好！请简单介绍一下你自己。")
        await asyncio.sleep(5)
        
        # 3. 发送另一个文本消息
        print("\n3️⃣  发送第二个文本消息...")
        await client.send_text("今天天气怎么样？")
        await asyncio.sleep(5)
        
        # 4. 模拟发送音频
        print("\n4️⃣  发送音频消息（模拟）...")
        await client.send_audio()
        await asyncio.sleep(3)
        
        print("\n✅ 测试完成！等待最后的响应...")
        await asyncio.sleep(3)
        
    except KeyboardInterrupt:
        print("\n⚠️  用户中断测试")
    except Exception as e:
        print(f"\n❌ 测试过程中出错: {e}")
    finally:
        # 取消接收任务
        receive_task.cancel()
        try:
            await receive_task
        except asyncio.CancelledError:
            pass
        
        # 断开连接
        await client.disconnect()

if __name__ == "__main__":
    asyncio.run(main())