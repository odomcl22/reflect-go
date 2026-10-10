import React from 'react';
import {AbsoluteFill, Img, Easing, interpolate, staticFile, useCurrentFrame, useVideoConfig} from 'remotion';

export const Frame: React.FC<{children: React.ReactNode}> = ({children}) => (
  <AbsoluteFill style={{backgroundColor:'#101a14',color:'#f3f7f3',fontFamily:'Arial, Helvetica, sans-serif'}}>
    <div style={{position:'absolute',inset:0,background:'radial-gradient(ellipse at 90% 0%, #254131 0%, transparent 62%)'}} />
    <div style={{position:'absolute',left:80,top:92,fontSize:32,fontWeight:700,letterSpacing:-0.6,color:'#b8dbbe'}}>Reflect Go</div>
    {children}
  </AbsoluteFill>
);

export const Screen: React.FC<{src:string;top:number;height:number;zoom?:number;x?:number;y?:number;still?:boolean}> = ({src,top,height,zoom=1,x=0,y=0,still=false}) => {
 const frame=useCurrentFrame(); const {fps}=useVideoConfig();
 return <div style={{position:'absolute',left:80,top,width:920,height,overflow:'hidden',borderRadius:24,border:'1px solid #435448',boxShadow:'0 26px 80px #0006',opacity:still ? 1 : interpolate(frame,[0,0.4*fps],[0,1],{extrapolateLeft:'clamp',extrapolateRight:'clamp'}),translate:still ? "0px 0px" : interpolate(frame,[0,0.6*fps],['0px 22px','0px 0px'],{easing:Easing.bezier(0.16,1,0.3,1),extrapolateLeft:'clamp',extrapolateRight:'clamp'})}}>
   <Img src={staticFile(src)} premountFor={fps} style={{position:'absolute',left:x,top:y,width:920*zoom,height:920*zoom*760/1040}} />
 </div>;
};
