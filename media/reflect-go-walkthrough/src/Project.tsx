import {Frame, Screen} from './Frame';
export const Project = () => <Frame>
 <div style={{position:'absolute',left:80,top:166,fontSize:84,fontWeight:700,letterSpacing:-3}}>Choose a project.</div>
 <div style={{position:'absolute',left:84,top:273,fontSize:44,color:'#c9d5cc',letterSpacing:-1}}>Optional. Open where you work.</div>
 <Screen src="project.jpg" top={360} height={600} />
</Frame>;
