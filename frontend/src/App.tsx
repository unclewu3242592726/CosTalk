import React, { useState, useRef, useEffect } from 'react';
import './App.css';

// WebSocket frame types
interface WSFrame {
  type: string;
  seq?: number;
  content?: any;
}

interface TextDeltaFrame {
  text: string;
}

interface AudioChunkFrame {
  format: string;
  data: string; // base64
}

interface Role {
  id: string;
  name: string;
  description: string;
}

function App() {
  const [isConnected, setIsConnected] = useState(false);
  const [currentRole, setCurrentRole] = useState<Role | null>(null);
  const [roles, setRoles] = useState<Role[]>([]);
  const [isRecording, setIsRecording] = useState(false);
  const [messages, setMessages] = useState<string[]>([]);
  
  const wsRef = useRef<WebSocket | null>(null);
  const audioContextRef = useRef<AudioContext | null>(null);
  const audioQueueRef = useRef<AudioBuffer[]>([]);
  const isPlayingRef = useRef(false);

  // Load roles on component mount
  useEffect(() => {
    fetchRoles();
  }, []);

  const fetchRoles = async () => {
    try {
      const response = await fetch('http://localhost:8888/v1/roles');
      const data = await response.json();
      setRoles(data.roles || []);
    } catch (error) {
      console.error('Failed to fetch roles:', error);
    }
  };

  const initAudioContext = async () => {
    if (!audioContextRef.current) {
      audioContextRef.current = new AudioContext();
      // Resume AudioContext (required by browser policy)
      if (audioContextRef.current.state === 'suspended') {
        await audioContextRef.current.resume();
      }
    }
  };

  const connectWebSocket = (roleId: string) => {
    if (wsRef.current) {
      wsRef.current.close();
    }

    const wsUrl = `ws://localhost:8888/v1/chat/stream?roleId=${roleId}`;
    wsRef.current = new WebSocket(wsUrl);

    wsRef.current.onopen = () => {
      setIsConnected(true);
      console.log('WebSocket connected');
    };

    wsRef.current.onmessage = (event) => {
      try {
        const frame: WSFrame = JSON.parse(event.data);
        handleWSFrame(frame);
      } catch (error) {
        console.error('Failed to parse WebSocket message:', error);
      }
    };

    wsRef.current.onclose = () => {
      setIsConnected(false);
      console.log('WebSocket disconnected');
    };

    wsRef.current.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
  };

  const handleWSFrame = async (frame: WSFrame) => {
    switch (frame.type) {
      case 'text_delta':
        const textFrame = frame.content as TextDeltaFrame;
        setMessages(prev => {
          const newMessages = [...prev];
          if (newMessages.length === 0 || !newMessages[newMessages.length - 1].startsWith('AI: ')) {
            newMessages.push('AI: ' + textFrame.text);
          } else {
            newMessages[newMessages.length - 1] += textFrame.text;
          }
          return newMessages;
        });
        break;
      
      case 'audio_chunk':
        const audioFrame = frame.content as AudioChunkFrame;
        await playAudioChunk(audioFrame);
        break;
      
      case 'end':
        console.log('Conversation ended');
        break;
      
      case 'error':
        console.error('Server error:', frame.content);
        break;
    }
  };

  const playAudioChunk = async (audioFrame: AudioChunkFrame) => {
    if (!audioContextRef.current) return;

    try {
      // Decode base64 audio data
      const audioData = atob(audioFrame.data);
      const audioArrayBuffer = new ArrayBuffer(audioData.length);
      const audioArray = new Uint8Array(audioArrayBuffer);
      
      for (let i = 0; i < audioData.length; i++) {
        audioArray[i] = audioData.charCodeAt(i);
      }

      // Decode audio buffer
      const audioBuffer = await audioContextRef.current.decodeAudioData(audioArrayBuffer);
      audioQueueRef.current.push(audioBuffer);

      if (!isPlayingRef.current) {
        playNextAudioBuffer();
      }
    } catch (error) {
      console.error('Failed to play audio chunk:', error);
    }
  };

  const playNextAudioBuffer = () => {
    if (audioQueueRef.current.length === 0 || !audioContextRef.current) {
      isPlayingRef.current = false;
      return;
    }

    isPlayingRef.current = true;
    const audioBuffer = audioQueueRef.current.shift()!;
    const source = audioContextRef.current.createBufferSource();
    source.buffer = audioBuffer;
    source.connect(audioContextRef.current.destination);

    source.onended = () => {
      playNextAudioBuffer();
    };

    source.start();
  };

  const selectRole = async (role: Role) => {
    setCurrentRole(role);
    await initAudioContext();
    connectWebSocket(role.id);
  };

  const startRecording = () => {
    // TODO: Implement actual recording
    setIsRecording(true);
    console.log('Recording started (placeholder)');
  };

  const stopRecording = () => {
    setIsRecording(false);
    console.log('Recording stopped (placeholder)');
    
    // Simulate sending a message
    const testMessage = "你好，我想和你聊天";
    setMessages(prev => [...prev, `User: ${testMessage}`]);
    
    // TODO: Send actual audio data to WebSocket
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({
        type: 'user_message',
        content: { text: testMessage }
      }));
    }
  };

  return (
    <div className="App">
      <header className="App-header">
        <h1>CosTalk - AI 角色对话</h1>
        
        {!currentRole ? (
          <div className="role-selection">
            <h2>选择一个角色开始对话</h2>
            <div className="roles-grid">
              {roles.map(role => (
                <div 
                  key={role.id} 
                  className="role-card"
                  onClick={() => selectRole(role)}
                >
                  <h3>{role.name}</h3>
                  <p>{role.description}</p>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <div className="chat-interface">
            <div className="chat-header">
              <h2>正在与 {currentRole.name} 对话</h2>
              <p>连接状态: {isConnected ? '已连接' : '未连接'}</p>
              <button onClick={() => setCurrentRole(null)}>切换角色</button>
            </div>
            
            <div className="messages">
              {messages.map((msg, index) => (
                <div key={index} className="message">
                  {msg}
                </div>
              ))}
            </div>
            
            <div className="voice-controls">
              <button 
                onMouseDown={startRecording}
                onMouseUp={stopRecording}
                onTouchStart={startRecording}
                onTouchEnd={stopRecording}
                className={`record-btn ${isRecording ? 'recording' : ''}`}
                disabled={!isConnected}
              >
                {isRecording ? '正在录音...' : '按住说话'}
              </button>
            </div>
          </div>
        )}
      </header>
    </div>
  );
}

export default App;
