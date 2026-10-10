import {Frame, Screen} from './Frame';
export const Client = () => <Frame>
 <div style={{position:'absolute',left:80,top:166,fontSize:84,fontWeight:700,letterSpacing:-3}}>Open your client.</div>
 <div style={{position:'absolute',left:84,top:280,fontSize:44,color:'#c9d5cc',letterSpacing:-1}}>Codex · Claude Code · OpenCode</div>
 <Screen src="home.jpg" top={394} height={465} zoom={1.28} x={-246} y={-126} />
 <div style={{position:'absolute',left:84,top:917,fontSize:40,color:'#b8dbbe',letterSpacing:-0.5}}>Your chosen models. Ready for your app.</div>
</Frame>;
