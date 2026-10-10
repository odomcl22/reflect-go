import {interpolate, useCurrentFrame, useVideoConfig} from 'remotion';
import {Frame} from './Frame';
export const Closing = () => {
 const frame=useCurrentFrame(); const {fps}=useVideoConfig();
 return <Frame>
   <div style={{position:'absolute',left:80,top:260,fontSize:128,fontWeight:700,lineHeight:1.03,letterSpacing:-5,opacity:interpolate(frame,[0,0.4*fps],[0,1],{extrapolateLeft:'clamp',extrapolateRight:'clamp'})}}>Bring your<br /><span style={{color:'#b8dbbe'}}>weights.</span></div>
   <div style={{position:'absolute',left:84,top:588,fontSize:44,color:'#c9d5cc',letterSpacing:-1}}>Explore Reflect Go.</div>
   <div style={{position:'absolute',left:84,top:706,fontSize:44,fontWeight:600,letterSpacing:-1,color:'#b8dbbe'}}>github.com/odomcl22/reflect-go</div>
   <div style={{position:'absolute',left:84,top:936,fontSize:32,color:'#9bb5a1'}}>Early preview · Bring your own model</div>
 </Frame>;
};
