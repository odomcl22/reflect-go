import {interpolate, useCurrentFrame, useVideoConfig} from 'remotion';
import {Frame, Screen} from './Frame';
export const Opening = ({still=false}: {still?:boolean}) => {
 const frame=useCurrentFrame(); const {fps}=useVideoConfig();
 return <Frame>
   <div style={{position:'absolute',left:80,top:172,fontSize:104,fontWeight:700,lineHeight:1.04,letterSpacing:-5,opacity:still ? 1 : interpolate(frame,[0,0.4*fps],[0,1],{extrapolateLeft:'clamp',extrapolateRight:'clamp'})}}>Your AI.<br /><span style={{color:'#b8dbbe'}}>On your drive.</span></div>
   <div style={{position:'absolute',left:84,top:410,fontSize:44,color:'#c9d5cc',letterSpacing:-1}}>A portable inference engine.</div>
   <Screen still={still} src="home.jpg" top={508} height={390} />
   <div style={{position:'absolute',left:84,top:938,fontSize:32,color:'#9bb5a1'}}>Early preview · Actual interface</div>
 </Frame>;
};
